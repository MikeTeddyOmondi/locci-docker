package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	return newTestServerWithKey(t, nil)
}

// newTestServerWithKey is newTestServer with LOCCI_SECRET_KEY set (box != nil).
func newTestServerWithKey(t *testing.T, box *secret.Box) (*httptest.Server, *store.Store) {
	t.Helper()
	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// The default environment points where nothing listens: "Docker down".
	if _, err := db.CreateEnvironment(store.Environment{Name: "local", Kind: store.KindLocal, URL: "tcp://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	envs := &env.Registry{Store: db, Box: box, Domain: "example.test", Log: log}
	t.Cleanup(envs.Close)

	s := &Server{Envs: envs, Store: db, AuthToken: "static", Log: log}
	ts := httptest.NewServer(s.Router("test"))
	t.Cleanup(ts.Close)
	return ts, db
}

func call(t *testing.T, method, url, token string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestAuthFlow(t *testing.T) {
	ts, db := newTestServer(t)
	if _, err := db.CreateUser("ann", "password123", "user"); err != nil {
		t.Fatal(err)
	}

	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me", "", nil); code != 401 {
		t.Fatalf("no token: got %d", code)
	}
	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me", "nope", nil); code != 401 {
		t.Fatalf("bad token: got %d", code)
	}
	if code, out := call(t, "GET", ts.URL+"/api/v1/auth/me", "static", nil); code != 200 || out["role"] != "admin" {
		t.Fatalf("static token: %d %v", code, out)
	}

	if code, _ := call(t, "POST", ts.URL+"/api/v1/auth/login", "", map[string]string{"username": "ann", "password": "wrong-pass"}); code != 401 {
		t.Fatalf("wrong password: got %d", code)
	}
	code, out := call(t, "POST", ts.URL+"/api/v1/auth/login", "", map[string]string{"username": "ann", "password": "password123"})
	if code != 200 {
		t.Fatalf("login: %d %v", code, out)
	}
	tok := out["token"].(string)

	if code, out := call(t, "GET", ts.URL+"/api/v1/auth/me", tok, nil); code != 200 || out["username"] != "ann" {
		t.Fatalf("me: %d %v", code, out)
	}
	// ?token= works too (used by the WebSocket terminal)
	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me?token="+tok, "", nil); code != 200 {
		t.Fatalf("query token: got %d", code)
	}
	// non-admins can't manage users
	if code, _ := call(t, "GET", ts.URL+"/api/v1/users", tok, nil); code != 403 {
		t.Fatalf("users as non-admin: got %d", code)
	}
	if code, _ := call(t, "POST", ts.URL+"/api/v1/users", "static", map[string]string{"username": "bob", "password": "password456"}); code != 201 {
		t.Fatalf("create user: got %d", code)
	}

	code, out = call(t, "GET", ts.URL+"/api/v1/audit", "static", nil)
	if code != 200 || len(out["audit"].([]any)) < 2 {
		t.Fatalf("audit: %d %v", code, out)
	}

	if code, _ := call(t, "POST", ts.URL+"/api/v1/auth/logout", tok, nil); code != 200 {
		t.Fatalf("logout: got %d", code)
	}
	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me", tok, nil); code != 401 {
		t.Fatalf("token still valid after logout: got %d", code)
	}
}

func TestDockerDownReturnsError(t *testing.T) {
	ts, _ := newTestServer(t)
	if code, _ := call(t, "GET", ts.URL+"/health", "", nil); code != 503 {
		t.Fatalf("health: got %d", code)
	}
	if code, out := call(t, "GET", ts.URL+"/api/v1/containers", "static", nil); code != 500 || out["error"] == nil {
		t.Fatalf("containers: %d %v", code, out)
	}
}
