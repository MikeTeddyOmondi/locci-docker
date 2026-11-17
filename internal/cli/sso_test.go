package cli

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/server"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso/ssotest"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// The CLI's loopback flow end to end: the test plays the browser.
func TestSSOLoginLoopback(t *testing.T) {
	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	k := make([]byte, 32)
	rand.Read(k)
	box, _ := secret.New(base64.StdEncoding.EncodeToString(k))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	idp := ssotest.New(t)
	sealed, _ := box.Seal(idp.ClientSecret)
	if _, err := db.CreateProvider(store.Provider{Slug: "fake", IssuerURL: idp.URL, ClientID: idp.ClientID,
		ClientSecret: sealed, Enabled: true, AutoCreateUsers: true}); err != nil {
		t.Fatal(err)
	}
	idp.SetUser(map[string]any{"sub": "u-1", "email": "lee@example.com", "email_verified": true})

	srv := &server.Server{Envs: &env.Registry{Store: db, Log: log}, SSO: &sso.Service{Store: db, Box: box, Log: log}, Store: db, Log: log}
	ts := httptest.NewServer(srv.Router("test"))
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	openURL = func(u string) error {
		go func() {
			if resp, err := browser.Get(u); err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	defer func() { openURL = openBrowser }()

	token, err := ssoLogin(client.Config{URL: ts.URL}, "fake")
	if err != nil {
		t.Fatal(err)
	}
	var me struct{ Username string }
	if err := client.New(client.Config{URL: ts.URL, Token: token}).Do("GET", "/api/v1/auth/me", nil, &me); err != nil || me.Username != "lee@example.com" {
		t.Fatalf("token from the loopback flow: %v %+v", err, me)
	}

	// A refused sign-in comes back as an error.
	idp.Deny()
	if _, err := ssoLogin(client.Config{URL: ts.URL}, "fake"); err == nil {
		t.Fatal("expected the denied sign-in to fail")
	}
}
