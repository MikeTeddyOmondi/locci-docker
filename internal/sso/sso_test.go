package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso/ssotest"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

const redirect = "http://locci.test/api/v1/auth/oidc/fake/callback"

type fixture struct {
	svc *Service
	idp *ssotest.Provider
	p   store.Provider
}

func setup(t *testing.T, mutate func(*store.Provider)) fixture {
	t.Helper()
	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	k := make([]byte, 32)
	rand.Read(k)
	box, _ := secret.New(base64.StdEncoding.EncodeToString(k))

	idp := ssotest.New(t)
	sealed, _ := box.Seal(idp.ClientSecret)
	p := store.Provider{Slug: "fake", IssuerURL: idp.URL, ClientID: idp.ClientID, ClientSecret: sealed,
		Enabled: true, AutoCreateUsers: true, AdminGroup: "locci-admins"}
	if mutate != nil {
		mutate(&p)
	}
	if p, err = db.CreateProvider(p); err != nil {
		t.Fatal(err)
	}
	return fixture{svc: &Service{Store: db, Box: box, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, idp: idp, p: p}
}

// login runs Start, the provider's authorize redirect and Finish.
func (f fixture) login(t *testing.T) (Claims, error) {
	t.Helper()
	ctx := context.Background()
	authURL, cookie, err := f.svc.Start(ctx, f.p, redirect, 0)
	if err != nil {
		t.Fatal(err)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(authURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	back, _ := url.Parse(resp.Header.Get("Location"))
	if e := back.Query().Get("error"); e != "" {
		return Claims{}, errors.New(e)
	}
	flow, err := f.svc.OpenFlow(cookie)
	if err != nil {
		t.Fatal(err)
	}
	return f.svc.Finish(ctx, f.p, redirect, flow, back.Query().Get("state"), back.Query().Get("code"))
}

func TestLoginCreatesUserAndSyncsRole(t *testing.T) {
	f := setup(t, nil)
	f.idp.SetUser(map[string]any{"sub": "u-1", "email": "Ann@Example.com", "email_verified": true, "groups": []string{"/locci-admins"}})

	c, err := f.login(t)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "u-1" || c.Email != "ann@example.com" || !c.EmailVerified {
		t.Fatalf("claims: %+v", c)
	}
	u, err := f.svc.Resolve(f.p, c)
	if err != nil || u.Username != "ann@example.com" || u.Role != "admin" || u.HasPassword {
		t.Fatalf("first login: %v %+v", err, u)
	}

	// Same subject, no longer in the admin group: same user, demoted.
	f.idp.SetUser(map[string]any{"sub": "u-1", "email": "ann@example.com", "email_verified": true, "groups": []string{"dev"}})
	c, _ = f.login(t)
	u2, err := f.svc.Resolve(f.p, c)
	if err != nil || u2.ID != u.ID || u2.Role != "user" {
		t.Fatalf("second login: %v %+v", err, u2)
	}
}

func TestFlowRejections(t *testing.T) {
	f := setup(t, nil)
	f.idp.SetUser(map[string]any{"sub": "u-1"})
	ctx := context.Background()

	_, cookie, _ := f.svc.Start(ctx, f.p, redirect, 0)
	flow, _ := f.svc.OpenFlow(cookie)
	if _, err := f.svc.Finish(ctx, f.p, redirect, flow, "wrong-state", "code"); err == nil {
		t.Fatal("expected a state mismatch to fail")
	}
	if _, err := f.svc.OpenFlow(cookie + "x"); err == nil {
		t.Fatal("expected a tampered cookie to fail")
	}

	// An expired flow is refused even though the cookie is genuine.
	raw, _ := json.Marshal(Flow{Slug: "fake", State: "s", Expires: time.Now().Add(-time.Minute).Unix()})
	old, _ := f.svc.Box.Seal(string(raw))
	if _, err := f.svc.OpenFlow(old); err == nil {
		t.Fatal("expected an expired flow to fail")
	}

	// The provider refusing the login.
	f.idp.Deny()
	if _, err := f.login(t); err == nil || err.Error() != "access_denied" {
		t.Fatalf("expected access_denied, got %v", err)
	}

	// A wrong client secret fails at the code exchange.
	bad, _ := f.svc.Box.Seal("not-the-secret")
	f.p.ClientSecret = bad
	f.idp.SetUser(map[string]any{"sub": "u-1"})
	if _, err := f.login(t); err == nil {
		t.Fatal("expected a wrong client secret to fail")
	}
}

func TestResolveRules(t *testing.T) {
	f := setup(t, func(p *store.Provider) {
		p.AutoCreateUsers = false
		p.AllowedDomains = "example.com"
		p.AdminGroup = ""
	})

	// Domain allow-list.
	if _, err := f.svc.Resolve(f.p, Claims{Subject: "x", Email: "eve@evil.com", EmailVerified: true}); err == nil {
		t.Fatal("expected a foreign domain to be refused")
	}
	if _, err := f.svc.Resolve(f.p, Claims{Subject: "x", Email: "eve@example.com", EmailVerified: false}); err == nil {
		t.Fatal("expected an unverified email to be refused when domains are restricted")
	}

	// No automatic accounts and no match.
	if _, err := f.svc.Resolve(f.p, Claims{Subject: "x", Email: "bob@example.com", EmailVerified: true}); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("expected ErrNoAccount, got %v", err)
	}

	// An existing account named after the verified email gets linked.
	existing, _ := f.svc.Store.CreateUser("bob@example.com", "password123", "user")
	u, err := f.svc.Resolve(f.p, Claims{Subject: "x", Email: "bob@example.com", EmailVerified: true})
	if err != nil || u.ID != existing.ID {
		t.Fatalf("link by email: %v %+v", err, u)
	}
	if again, _ := f.svc.Store.UserByIdentity(f.p.ID, "x"); again.ID != existing.ID {
		t.Fatal("identity should now be linked")
	}

	// An unverified email never links to an existing account.
	f.p.AllowedDomains, f.p.AutoCreateUsers = "", true
	f.svc.Store.CreateUser("admin", "password123", "admin")
	u, err = f.svc.Resolve(f.p, Claims{Subject: "y", PreferredUsername: "admin"})
	if err != nil || u.Username != "admin-2" || u.IsAdmin() {
		t.Fatalf("clash handling: %v %+v", err, u)
	}
}

func TestGroupsFromUserinfo(t *testing.T) {
	f := setup(t, nil)
	f.idp.SetUser(map[string]any{"sub": "u-9"}) // no email or groups in the ID token
	f.idp.SetUserInfo(map[string]any{"email": "kim@example.com", "email_verified": "true", "groups": "locci-admins"})
	c, err := f.login(t)
	if err != nil {
		t.Fatal(err)
	}
	if c.Email != "kim@example.com" || !c.EmailVerified || len(c.Groups) != 1 {
		t.Fatalf("userinfo merge: %+v", c)
	}
	if u, _ := f.svc.Resolve(f.p, c); u.Role != "admin" {
		t.Fatalf("expected admin from userinfo groups, got %+v", u)
	}
}

func TestClaimsShapes(t *testing.T) {
	// Zitadel: roles as an object keyed by role name.
	c := claimsFrom(map[string]any{"urn:zitadel:iam:org:project:roles": map[string]any{
		"locci-admins": map[string]any{"123": "org.example.com"}, "dev": map[string]any{},
	}}, "urn:zitadel:iam:org:project:roles")
	if len(c.Groups) != 2 || c.Groups[0] != "dev" || c.Groups[1] != "locci-admins" {
		t.Fatalf("object groups: %v", c.Groups)
	}
	if role(store.Provider{AdminGroup: "locci-admins", DefaultRole: "user"}, c) != "admin" {
		t.Fatal("expected a role key to grant admin")
	}
	// A string email_verified and a single-string group.
	c = claimsFrom(map[string]any{"email_verified": "true", "groups": "ops"}, "groups")
	if !c.EmailVerified || len(c.Groups) != 1 {
		t.Fatalf("string shapes: %+v", c)
	}
	// No email_verified claim (Entra ID) means unverified.
	if claimsFrom(map[string]any{"email": "a@b.c"}, "groups").EmailVerified {
		t.Fatal("a missing email_verified must count as unverified")
	}
}
