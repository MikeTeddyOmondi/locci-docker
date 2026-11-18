// Package sso signs users in through OpenID Connect providers.
//
// The flow is the authorization code flow with PKCE:
//
//  1. Start creates state, nonce and a PKCE verifier, seals them (with the
//     LOCCI_SECRET_KEY box) into a short-lived cookie and returns the
//     provider's authorization URL.
//  2. Finish opens the cookie, checks state, exchanges the code with the
//     verifier, verifies the ID token (issuer, audience, expiry, nonce) and
//     returns the user's claims.
//  3. Resolve maps the claims to a locci user: an already linked identity, an
//     existing account whose username is the verified email, or (when the
//     provider allows it) a new SSO-only account.
//
// Nothing here writes HTTP responses; internal/server does that.
package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// FlowTTL is how long a user has to finish signing in at the provider.
const FlowTTL = 10 * time.Minute

// ErrNoAccount means the identity is valid but locci has no user for it and
// the provider doesn't create users automatically.
var ErrNoAccount = errors.New("no locci account is linked to this identity; ask an admin to create one or to enable automatic accounts")

type Service struct {
	Store *store.Store
	Box   *secret.Box  // seals the flow cookie and opens client secrets; required
	HTTP  *http.Client // for discovery, token and userinfo calls; nil = 10s default
	Log   *slog.Logger

	mu    sync.Mutex
	cache map[string]cached // issuer -> discovery result
}

type cached struct {
	p  *oidc.Provider
	at time.Time
}

// Claims are the parts of the identity locci uses.
type Claims struct {
	Subject           string   `json:"sub"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
}

// Flow is what the sealed cookie carries between Start and Finish.
type Flow struct {
	Slug     string `json:"p"`
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	CLIPort  int    `json:"c,omitempty"` // CLI loopback port, 0 for the browser
	Expires  int64  `json:"e"`
}

func (s *Service) ctx(ctx context.Context) context.Context {
	c := s.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	return oidc.ClientContext(ctx, c)
}

// Discover fetches (and caches for an hour) the provider's configuration.
func (s *Service) Discover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.mu.Lock()
	if c, ok := s.cache[issuer]; ok && time.Since(c.at) < time.Hour {
		s.mu.Unlock()
		return c.p, nil
	}
	s.mu.Unlock()

	p, err := oidc.NewProvider(s.ctx(ctx), issuer)
	if err != nil {
		return nil, fmt.Errorf("discovery for %s: %w", issuer, err)
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]cached{}
	}
	s.cache[issuer] = cached{p: p, at: time.Now()}
	s.mu.Unlock()
	return p, nil
}

// Forget drops a cached discovery result (after an edit or an error).
func (s *Service) Forget(issuer string) {
	s.mu.Lock()
	delete(s.cache, issuer)
	s.mu.Unlock()
}

func (s *Service) config(ctx context.Context, p store.Provider, redirectURL string) (*oauth2.Config, *oidc.Provider, error) {
	op, err := s.Discover(ctx, p.IssuerURL)
	if err != nil {
		return nil, nil, err
	}
	clientSecret, err := s.Box.Open(p.ClientSecret)
	if err != nil {
		return nil, nil, err
	}
	scopes := strings.Fields(p.Scopes)
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: clientSecret,
		Endpoint:     op.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}, op, nil
}

// Start returns the provider URL to send the user to and the sealed cookie.
func (s *Service) Start(ctx context.Context, p store.Provider, redirectURL string, cliPort int) (authURL, cookie string, err error) {
	if s.Box == nil {
		return "", "", secret.ErrNoKey
	}
	cfg, _, err := s.config(ctx, p, redirectURL)
	if err != nil {
		return "", "", err
	}
	f := Flow{
		Slug: p.Slug, State: oauth2.GenerateVerifier(), Nonce: oauth2.GenerateVerifier(),
		Verifier: oauth2.GenerateVerifier(), CLIPort: cliPort, Expires: time.Now().Add(FlowTTL).Unix(),
	}
	raw, _ := json.Marshal(f)
	if cookie, err = s.Box.Seal(string(raw)); err != nil {
		return "", "", err
	}
	authURL = cfg.AuthCodeURL(f.State, oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier))
	return authURL, cookie, nil
}

// OpenFlow reads the sealed cookie (also used to find where to send errors).
func (s *Service) OpenFlow(cookie string) (Flow, error) {
	var f Flow
	if s.Box == nil {
		return f, secret.ErrNoKey
	}
	raw, err := s.Box.Open(cookie)
	if err != nil {
		return f, errors.New("sign-in session is missing or invalid; start again")
	}
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return f, errors.New("sign-in session is invalid; start again")
	}
	if time.Now().Unix() > f.Expires {
		return f, errors.New("sign-in took too long; start again")
	}
	return f, nil
}

// Finish completes the flow and returns the verified claims.
func (s *Service) Finish(ctx context.Context, p store.Provider, redirectURL string, f Flow, state, code string) (Claims, error) {
	if f.Slug != p.Slug || state == "" || state != f.State {
		return Claims{}, errors.New("sign-in state doesn't match; start again")
	}
	cfg, op, err := s.config(ctx, p, redirectURL)
	if err != nil {
		return Claims{}, err
	}
	octx := s.ctx(ctx)
	tok, err := cfg.Exchange(octx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		s.Forget(p.IssuerURL)
		return Claims{}, fmt.Errorf("code exchange: %w", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	return s.verifiedClaims(octx, p, op, tok, rawID, f.Nonce)
}

// verifiedClaims checks an ID token (issuer, audience, expiry, and the nonce
// when one was sent) and returns its claims, filling email and groups from
// userinfo when the token lacks them. Shared by the browser and device flows.
func (s *Service) verifiedClaims(octx context.Context, p store.Provider, op *oidc.Provider, tok *oauth2.Token, rawID, nonce string) (Claims, error) {
	if rawID == "" {
		return Claims{}, errors.New("the provider returned no ID token (is the openid scope allowed?)")
	}
	idt, err := op.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(octx, rawID)
	if err != nil {
		return Claims{}, fmt.Errorf("ID token: %w", err)
	}
	if nonce != "" && idt.Nonce != nonce {
		return Claims{}, errors.New("ID token nonce doesn't match; start again")
	}

	all := map[string]any{}
	if err := idt.Claims(&all); err != nil {
		return Claims{}, err
	}
	c := claimsFrom(all, p.GroupsClaim)
	c.Subject = idt.Subject

	// Some providers only put email or groups in userinfo.
	if (c.Email == "" || len(c.Groups) == 0) && tok.AccessToken != "" {
		if ui, err := op.UserInfo(octx, oauth2.StaticTokenSource(tok)); err == nil && ui.Subject == c.Subject {
			more := map[string]any{}
			if ui.Claims(&more) == nil {
				extra := claimsFrom(more, p.GroupsClaim)
				if c.Email == "" {
					c.Email, c.EmailVerified = extra.Email, extra.EmailVerified
				}
				if len(c.Groups) == 0 {
					c.Groups = extra.Groups
				}
			}
		}
	}
	return c, nil
}

// claimsFrom reads claims leniently: email_verified may be a bool or a
// string, and groups a list or a single string.
func claimsFrom(m map[string]any, groupsClaim string) Claims {
	str := func(k string) string { v, _ := m[k].(string); return v }
	c := Claims{
		Email: strings.ToLower(strings.TrimSpace(str("email"))), Name: str("name"),
		PreferredUsername: str("preferred_username"),
	}
	switch v := m["email_verified"].(type) {
	case bool:
		c.EmailVerified = v
	case string:
		c.EmailVerified = v == "true"
	}
	switch v := m[groupsClaim].(type) {
	case []any:
		for _, g := range v {
			if s, ok := g.(string); ok {
				c.Groups = append(c.Groups, s)
			}
		}
	case string:
		c.Groups = []string{v}
	}
	return c
}

// role returns admin for members of the provider's admin group (Keycloak
// may prefix group paths with "/"), otherwise the default role.
func role(p store.Provider, c Claims) string {
	if p.AdminGroup != "" {
		want := strings.TrimPrefix(p.AdminGroup, "/")
		for _, g := range c.Groups {
			if strings.TrimPrefix(g, "/") == want {
				return "admin"
			}
		}
	}
	return p.DefaultRole
}

func domainAllowed(p store.Provider, c Claims) error {
	domains := p.Domains()
	if len(domains) == 0 {
		return nil
	}
	if c.Email == "" || !c.EmailVerified {
		return errors.New("this provider only admits verified emails from allowed domains, and none was given")
	}
	at := strings.LastIndex(c.Email, "@")
	if at < 0 || !slices.Contains(domains, c.Email[at+1:]) {
		return fmt.Errorf("email domain of %s is not allowed", c.Email)
	}
	return nil
}

// Resolve finds or creates the locci user for verified claims. When the
// provider has an admin group, the role follows group membership on every
// sign-in.
func (s *Service) Resolve(p store.Provider, c Claims) (store.User, error) {
	if c.Subject == "" {
		return store.User{}, errors.New("the ID token has no subject")
	}
	if err := domainAllowed(p, c); err != nil {
		return store.User{}, err
	}

	if u, err := s.Store.UserByIdentity(p.ID, c.Subject); err == nil {
		return s.syncRole(p, c, u)
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}

	// An existing account whose username is this verified email: link it.
	if c.Email != "" && c.EmailVerified {
		if u, err := s.Store.UserByName(c.Email); err == nil {
			if err := s.Store.LinkIdentity(p.ID, c.Subject, u.ID, c.Email); err != nil {
				return store.User{}, err
			}
			return s.syncRole(p, c, u)
		}
	}

	if !p.AutoCreateUsers {
		return store.User{}, ErrNoAccount
	}
	u, err := s.createUser(p, c)
	if err != nil {
		return store.User{}, err
	}
	return u, s.Store.LinkIdentity(p.ID, c.Subject, u.ID, c.Email)
}

func (s *Service) syncRole(p store.Provider, c Claims, u store.User) (store.User, error) {
	if p.AdminGroup == "" {
		return u, nil
	}
	if r := role(p, c); r != u.Role {
		if err := s.Store.SetRole(u.ID, r); err != nil {
			return u, err
		}
		u.Role = r
	}
	return u, nil
}

// createUser picks a free username: the verified email, else the preferred
// username, else <provider>-<subject>, adding -2, -3... on clashes.
func (s *Service) createUser(p store.Provider, c Claims) (store.User, error) {
	base := ""
	switch {
	case c.Email != "" && c.EmailVerified:
		base = c.Email
	case c.PreferredUsername != "":
		base = strings.ToLower(c.PreferredUsername)
	default:
		sub := c.Subject
		if len(sub) > 12 {
			sub = sub[:12]
		}
		base = p.Slug + "-" + sub
	}
	name := base
	for i := 2; i < 100; i++ {
		if _, err := s.Store.UserByName(name); errors.Is(err, store.ErrNotFound) {
			return s.Store.CreateSSOUser(name, role(p, c))
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return store.User{}, errors.New("couldn't find a free username")
}
