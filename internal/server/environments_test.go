package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
)

func sshKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

func testBox(t *testing.T) *secret.Box {
	t.Helper()
	k := make([]byte, 32)
	rand.Read(k)
	box, err := secret.New(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestEnvironmentAPI(t *testing.T) {
	ts, db := newTestServerWithKey(t, testBox(t))
	api := ts.URL + "/api/v1"
	if _, err := db.CreateUser("ann", "password123", "user"); err != nil {
		t.Fatal(err)
	}
	_, out := call(t, "POST", api+"/auth/login", "", map[string]string{"username": "ann", "password": "password123"})
	ann := out["token"].(string)

	// Everyone can list; only admins can change.
	if code, out := call(t, "GET", api+"/environments", ann, nil); code != 200 || len(out["environments"].([]any)) != 1 {
		t.Fatalf("list as user: %d %v", code, out)
	}
	fake := map[string]any{"name": "fake", "kind": "local", "url": fakeDocker(t), "labels": map[string]string{"site": "lab"}}
	if code, _ := call(t, "POST", api+"/environments", ann, fake); code != 403 {
		t.Fatalf("create as user: got %d", code)
	}

	// Test before saving: reachable and unreachable hosts both answer 200.
	if code, out := call(t, "POST", api+"/environments/test", "static", fake); code != 200 || out["ok"] != true {
		t.Fatalf("test fake: %d %v", code, out)
	}
	dead := map[string]any{"kind": "local", "url": "tcp://127.0.0.1:1"}
	if code, out := call(t, "POST", api+"/environments/test", "static", dead); code != 200 || out["ok"] != false || out["error"] == "" {
		t.Fatalf("test dead: %d %v", code, out)
	}

	// Create, then use it.
	if code, out := call(t, "POST", api+"/environments", "static", fake); code != 201 || out["name"] != "fake" || out["has_secret"] != false {
		t.Fatalf("create: %d %v", code, out)
	}
	if code, _ := call(t, "GET", api+"/environments/fake/containers", ann, nil); code != 200 {
		t.Fatalf("use new env: got %d", code)
	}

	// Validation errors are 400s.
	for _, bad := range []map[string]any{
		{"name": "test", "kind": "local"},                                         // reserved
		{"name": "fake", "kind": "local"},                                         // duplicate
		{"name": "edge", "kind": "ssh", "url": "ssh://me@h", "ssh_key": "junk"},   // bad key
		{"name": "edge", "kind": "tcp+tls", "url": "tcp://h:2376", "tls_ca": "x"}, // missing cert/key
	} {
		if code, out := call(t, "POST", api+"/environments", "static", bad); code != 400 {
			t.Errorf("expected 400 for %v, got %d %v", bad["name"], code, out)
		}
	}

	// SSH: the key is stored encrypted and never comes back.
	keyPEM := sshKeyPEM(t)
	edge := map[string]any{"name": "edge", "kind": "ssh", "url": "ssh://deploy@127.0.0.1:1", "ssh_key": keyPEM}
	code, out := call(t, "POST", api+"/environments", "static", edge)
	if code != 201 || out["has_secret"] != true {
		t.Fatalf("create ssh: %d %v", code, out)
	}
	stored, _ := db.EnvironmentByName("edge")
	if !strings.HasPrefix(stored.Secret, "v1:") || strings.Contains(stored.Secret, "PRIVATE") {
		t.Fatalf("secret not encrypted at rest: %q", stored.Secret[:10])
	}
	for _, path := range []string{"/environments", "/environments/edge"} {
		req, _ := http.NewRequest("GET", api+path, nil)
		req.Header.Set("Authorization", "Bearer static")
		resp, _ := http.DefaultClient.Do(req)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(raw), "PRIVATE KEY") || strings.Contains(string(raw), stored.Secret) || strings.Contains(string(raw), "ssh_key") {
			t.Fatalf("%s leaks the secret: %s", path, raw)
		}
	}

	// Update: label only keeps the secret; kind can't change.
	if code, out := call(t, "PATCH", api+"/environments/edge", "static", map[string]any{"label": "Edge box"}); code != 200 || out["label"] != "Edge box" || out["has_secret"] != true {
		t.Fatalf("patch label: %d %v", code, out)
	}
	if code, _ := call(t, "PATCH", api+"/environments/edge", "static", map[string]any{"kind": "local"}); code != 400 {
		t.Fatalf("patch kind: got %d", code)
	}
	if code, _ := call(t, "PATCH", api+"/environments/nope", "static", map[string]any{"label": "x"}); code != 404 {
		t.Fatalf("patch missing: got %d", code)
	}

	// The default can't be deleted until another one is the default.
	if code, _ := call(t, "DELETE", api+"/environments/local", "static", nil); code != 409 {
		t.Fatalf("delete default: got %d", code)
	}
	if code, out := call(t, "POST", api+"/environments/fake/default", "static", nil); code != 200 || out["is_default"] != true {
		t.Fatalf("set default: %d %v", code, out)
	}
	if code, _ := call(t, "DELETE", api+"/environments/local", "static", nil); code != 200 {
		t.Fatalf("delete old default: got %d", code)
	}
	if code, _ := call(t, "GET", api+"/environments/local/containers", "static", nil); code != 404 {
		t.Fatalf("deleted env still routable: got %d", code)
	}
}

func TestEnvironmentNeedsSecretKey(t *testing.T) {
	ts, _ := newTestServer(t) // no LOCCI_SECRET_KEY
	edge := map[string]any{"name": "edge", "kind": "ssh", "url": "ssh://deploy@127.0.0.1:1", "ssh_key": sshKeyPEM(t)}
	code, out := call(t, "POST", ts.URL+"/api/v1/environments", "static", edge)
	if code != 412 || !strings.Contains(out["error"].(string), "LOCCI_SECRET_KEY") {
		t.Fatalf("expected 412 explaining the key, got %d %v", code, out)
	}
}
