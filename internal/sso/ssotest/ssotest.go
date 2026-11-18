// Package ssotest is a small fake OpenID Connect provider for tests.
//
// Its authorize endpoint "logs in" whoever SetUser configured and redirects
// straight back with a code; the token endpoint checks the client secret and
// the PKCE verifier and returns an RS256-signed ID token carrying the nonce.
package ssotest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type Provider struct {
	URL          string
	ClientID     string
	ClientSecret string

	key *rsa.PrivateKey

	mu       sync.Mutex
	claims   map[string]any // ID token claims for the next login
	userinfo map[string]any // extra claims served at /userinfo
	deny     bool
	codes    map[string]grant

	NoDevice bool               // hide the device endpoint from discovery
	devices  map[string]*device // device_code -> state
	lastDev  string
}

type device struct {
	approved, denied bool
	claims           map[string]any
}

type grant struct {
	nonce, challenge, redirect string
	claims                     map[string]any
}

// New starts a provider with client "locci" / secret "s3cret".
func New(t testing.TB) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{ClientID: "locci", ClientSecret: "s3cret", key: key, codes: map[string]grant{}, devices: map[string]*device{}}
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	p.URL = srv.URL
	return p
}

// SetUser sets the claims (sub, email, groups, ...) of the next sign-in.
func (p *Provider) SetUser(claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claims, p.deny = claims, false
}

// SetUserInfo sets claims only available from the userinfo endpoint.
func (p *Provider) SetUserInfo(claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.userinfo = claims
}

// ApproveDevice approves the latest device code as the SetUser user.
func (p *Provider) ApproveDevice() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if d := p.devices[p.lastDev]; d != nil {
		d.approved, d.claims = true, p.claims
	}
}

// DenyDevice refuses the latest device code.
func (p *Provider) DenyDevice() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if d := p.devices[p.lastDev]; d != nil {
		d.denied = true
	}
}

// Deny makes the next authorization fail with access_denied.
func (p *Provider) Deny() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deny = true
}

func random() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		doc := map[string]any{
			"issuer":                                p.URL,
			"authorization_endpoint":                p.URL + "/auth",
			"token_endpoint":                        p.URL + "/token",
			"jwks_uri":                              p.URL + "/jwks",
			"userinfo_endpoint":                     p.URL + "/userinfo",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"code_challenge_methods_supported":      []string{"S256"},
		}
		if !p.NoDevice {
			doc["device_authorization_endpoint"] = p.URL + "/device"
		}
		writeJSON(w, doc)
	case "/device":
		p.deviceAuth(w, r)
	case "/jwks":
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &p.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	case "/auth":
		p.authorize(w, r)
	case "/token":
		p.token(w, r)
	case "/userinfo":
		p.mu.Lock()
		info := map[string]any{"sub": p.claims["sub"]}
		for k, v := range p.userinfo {
			info[k] = v
		}
		p.mu.Unlock()
		writeJSON(w, info)
	default:
		http.NotFound(w, r)
	}
}

func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || q.Get("client_id") != p.ClientID || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "bad authorization request", http.StatusBadRequest)
		return
	}
	out := url.Values{"state": {q.Get("state")}}

	p.mu.Lock()
	if p.deny {
		out.Set("error", "access_denied")
		out.Set("error_description", "The user said no")
	} else {
		code := random()
		p.codes[code] = grant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: back.String(), claims: p.claims}
		out.Set("code", code)
	}
	p.mu.Unlock()

	back.RawQuery = out.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// clientOK checks client_secret_basic (form-decoded) or client_secret_post.
func (p *Provider) clientOK(r *http.Request) bool {
	id, sec, ok := r.BasicAuth()
	if ok {
		id, _ = url.QueryUnescape(id)
		sec, _ = url.QueryUnescape(sec)
	} else {
		id, sec = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	return id == p.ClientID && sec == p.ClientSecret
}

func (p *Provider) deviceAuth(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if !p.clientOK(r) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, map[string]string{"error": "invalid_client"})
		return
	}
	code := random()
	p.mu.Lock()
	p.devices[code] = &device{}
	p.lastDev = code
	p.mu.Unlock()
	writeJSON(w, map[string]any{
		"device_code": code, "user_code": "ABCD-EFGH", "verification_uri": p.URL + "/activate",
		"verification_uri_complete": p.URL + "/activate?code=ABCD-EFGH", "expires_in": 600, "interval": 1,
	})
}

// deviceGrant answers a device_code token request.
func (p *Provider) deviceGrant(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	d := p.devices[r.PostForm.Get("device_code")]
	p.mu.Unlock()
	fail := func(e string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": e})
	}
	switch {
	case d == nil:
		fail("invalid_grant")
	case d.denied:
		fail("access_denied")
	case !d.approved:
		fail("authorization_pending")
	default:
		p.issue(w, d.claims, "")
	}
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if !p.clientOK(r) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	if r.PostForm.Get("grant_type") == "urn:ietf:params:oauth:grant-type:device_code" {
		p.deviceGrant(w, r)
		return
	}
	p.mu.Lock()
	g, found := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code")) // codes are single use
	p.mu.Unlock()

	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case !found || r.PostForm.Get("redirect_uri") != g.redirect:
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	case base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge:
		http.Error(w, `{"error":"invalid_grant","error_description":"PKCE"}`, http.StatusBadRequest)
		return
	}

	p.issue(w, g.claims, g.nonce)
}

// issue writes a token response with an RS256 ID token for the claims.
func (p *Provider) issue(w http.ResponseWriter, user map[string]any, nonce string) {
	claims := map[string]any{
		"iss": p.URL, "aud": p.ClientID,
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	for k, v := range user {
		claims[k] = v
	}
	payload, _ := json.Marshal(claims)
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	jws, _ := signer.Sign(payload)
	idToken, _ := jws.CompactSerialize()

	writeJSON(w, map[string]any{
		"access_token": "at-" + random(), "token_type": "Bearer", "expires_in": 300, "id_token": idToken,
	})
}

// Origin returns scheme://host of the provider, handy for redirect checks.
func (p *Provider) Origin() string { return strings.TrimSuffix(p.URL, "/") }
