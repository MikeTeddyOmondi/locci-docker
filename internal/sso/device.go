package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// The device authorization flow (RFC 8628) for machines without a browser:
// the CLI shows a code, the user approves it on any device, and the CLI polls
// until the provider answers. locci talks to the provider (it holds the
// client secret); the CLI only sees the user code and a sealed handle.

var (
	// ErrPending: the user hasn't approved yet; poll again after the interval.
	ErrPending = errors.New("authorization_pending")
	// ErrSlowDown: polling too fast; add 5 seconds to the interval.
	ErrSlowDown = errors.New("slow_down")
	// ErrNoDeviceFlow: the provider's discovery has no device endpoint.
	ErrNoDeviceFlow = errors.New("this provider doesn't offer the device flow (no device_authorization_endpoint); enable it at the provider or use the browser sign-in")
)

// DeviceCode is what the CLI shows and keeps. Handle is sealed and opaque.
type DeviceCode struct {
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
	Handle                  string `json:"handle"`
}

type deviceHandle struct {
	Slug       string `json:"p"`
	DeviceCode string `json:"d"`
	Verifier   string `json:"v"` // PKCE verifier for the token request
	Expires    int64  `json:"e"`
}

// DeviceStart asks the provider for a device code.
func (s *Service) DeviceStart(ctx context.Context, p store.Provider) (DeviceCode, error) {
	if s.Box == nil {
		return DeviceCode{}, secret.ErrNoKey
	}
	cfg, _, err := s.config(ctx, p, "")
	if err != nil {
		return DeviceCode{}, err
	}
	if cfg.Endpoint.DeviceAuthURL == "" {
		return DeviceCode{}, ErrNoDeviceFlow
	}
	// Not oauth2's DeviceAuth: it sends no client secret, which confidential
	// clients (Keycloak, Authentik) need at this endpoint. PKCE is sent too:
	// Keycloak requires it here when the client enforces it, and providers
	// that don't use it for this flow ignore it.
	verifier := oauth2.GenerateVerifier()
	form := url.Values{
		"client_id":             {cfg.ClientID},
		"scope":                 {strings.Join(cfg.Scopes, " ")},
		"code_challenge":        {oauth2.S256ChallengeFromVerifier(verifier)},
		"code_challenge_method": {"S256"},
	}
	var da struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURL         string `json:"verification_url"` // Google's spelling
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
		Error                   string `json:"error"`
		Description             string `json:"error_description"`
	}
	status, err := s.postForm(s.ctx(ctx), cfg, cfg.Endpoint.DeviceAuthURL, form, &da)
	if err != nil {
		return DeviceCode{}, fmt.Errorf("device authorization: %w", err)
	}
	if da.Error != "" || status >= 300 || da.DeviceCode == "" {
		msg := da.Error
		if da.Description != "" {
			msg += ": " + da.Description
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return DeviceCode{}, fmt.Errorf("device authorization: %s", msg)
	}
	if da.VerificationURI == "" {
		da.VerificationURI = da.VerificationURL
	}
	expires := time.Now().Add(FlowTTL)
	if da.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(da.ExpiresIn) * time.Second)
	}
	raw, _ := json.Marshal(deviceHandle{Slug: p.Slug, DeviceCode: da.DeviceCode, Verifier: verifier, Expires: expires.Unix()})
	handle, err := s.Box.Seal(string(raw))
	if err != nil {
		return DeviceCode{}, err
	}
	interval := da.Interval
	if interval <= 0 {
		interval = 5
	}
	return DeviceCode{
		UserCode: da.UserCode, VerificationURI: da.VerificationURI, VerificationURIComplete: da.VerificationURIComplete,
		ExpiresIn: int64(time.Until(expires).Seconds()), Interval: interval, Handle: handle,
	}, nil
}

// DevicePoll asks the provider once whether the code was approved. It returns
// ErrPending or ErrSlowDown while waiting, and verified claims once approved.
func (s *Service) DevicePoll(ctx context.Context, p store.Provider, handle string) (Claims, error) {
	if s.Box == nil {
		return Claims{}, secret.ErrNoKey
	}
	raw, err := s.Box.Open(handle)
	var h deviceHandle
	if err != nil || json.Unmarshal([]byte(raw), &h) != nil || h.Slug != p.Slug {
		return Claims{}, errors.New("device sign-in handle is invalid; start again")
	}
	if time.Now().Unix() > h.Expires {
		return Claims{}, errors.New("the code expired; start again")
	}

	cfg, op, err := s.config(ctx, p, "")
	if err != nil {
		return Claims{}, err
	}
	octx := s.ctx(ctx)
	tok, rawID, err := s.deviceToken(octx, cfg, h.DeviceCode, h.Verifier)
	if err != nil {
		return Claims{}, err
	}
	return s.verifiedClaims(octx, p, op, tok, rawID, "")
}

// postForm sends a form to the provider with the client's credentials
// (client_secret_basic, form-encoded as RFC 6749 section 2.3.1 asks) and
// decodes the JSON answer, error responses included.
func (s *Service) postForm(octx context.Context, cfg *oauth2.Config, endpoint string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(octx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if cfg.ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape(cfg.ClientSecret))
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(body, out); err != nil {
		return resp.StatusCode, fmt.Errorf("unexpected response (%s)", resp.Status)
	}
	return resp.StatusCode, nil
}

// deviceToken makes one token request. oauth2's DeviceAccessToken polls in a
// loop until done, which doesn't fit a request/response API, so this is the
// single-shot version of it.
func (s *Service) deviceToken(octx context.Context, cfg *oauth2.Config, deviceCode, verifier string) (*oauth2.Token, string, error) {
	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
		"client_id":   {cfg.ClientID},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if _, err := s.postForm(octx, cfg, cfg.Endpoint.TokenURL, form, &out); err != nil {
		return nil, "", fmt.Errorf("device token: %w", err)
	}
	switch out.Error {
	case "":
	case "authorization_pending":
		return nil, "", ErrPending
	case "slow_down":
		return nil, "", ErrSlowDown
	case "access_denied":
		return nil, "", errors.New("the sign-in was denied")
	case "expired_token":
		return nil, "", errors.New("the code expired; start again")
	default:
		if out.Description != "" {
			return nil, "", fmt.Errorf("device token: %s: %s", out.Error, out.Description)
		}
		return nil, "", fmt.Errorf("device token: %s", out.Error)
	}
	return &oauth2.Token{AccessToken: out.AccessToken, TokenType: out.TokenType}, out.IDToken, nil
}
