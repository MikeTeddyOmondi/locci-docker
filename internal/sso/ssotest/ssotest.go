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
	p := &Provider{ClientID: "locci", ClientSecret: "s3cret", key: key, codes: map[string]grant{}}
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
		writeJSON(w, map[string]any{
			"issuer":                                p.URL,
			"authorization_endpoint":                p.URL + "/auth",
			"token_endpoint":                        p.URL + "/token",
			"jwks_uri":                              p.URL + "/jwks",
			"userinfo_endpoint":                     p.URL + "/userinfo",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"code_challenge_methods_supported":      []string{"S256"},
		})
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

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id, sec, ok := r.BasicAuth()
	if !ok {
		id, sec = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	p.mu.Lock()
	g, found := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code")) // codes are single use
	p.mu.Unlock()

	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case id != p.ClientID || sec != p.ClientSecret:
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	case !found || r.PostForm.Get("redirect_uri") != g.redirect:
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	case base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge:
		http.Error(w, `{"error":"invalid_grant","error_description":"PKCE"}`, http.StatusBadRequest)
		return
	}

	claims := map[string]any{
		"iss": p.URL, "aud": p.ClientID, "nonce": g.nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	for k, v := range g.claims {
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
