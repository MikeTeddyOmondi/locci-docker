package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/MikeTeddyOmondi/locci-docker/internal/tunnel"
)

// unixDocker serves a tiny Docker API on a unix socket, like a host's daemon.
func unixDocker(t *testing.T) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "ag")
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Id":"behind-nat","Names":["/edge-web"],"State":"running"}]`))
		default:
			http.NotFound(w, r)
		}
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

// runAgent connects like `locci agent` and serves sock until ctx ends.
func runAgent(t *testing.T, ctx context.Context, base, token, sock string) (string, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(base, "http") + "/api/v1/agent/connect"
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		return "", err
	}
	go tunnel.Serve(ctx, tunnel.NewConn(ws), func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return resp.Header.Get("X-Locci-Environment"), nil
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 150; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAgentEnvironment(t *testing.T) {
	ts, _ := newTestServer(t) // agent environments need no LOCCI_SECRET_KEY
	api := ts.URL + "/api/v1"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	code, out := call(t, "POST", api+"/environments", "static", map[string]any{"name": "edge", "kind": "agent"})
	if code != 201 || out["agent_connected"] != false {
		t.Fatalf("create: %d %v", code, out)
	}
	token, _ := out["agent_token"].(string)
	if !strings.HasPrefix(token, "la_") {
		t.Fatalf("expected an agent token once, got %v", out)
	}
	// It is never shown again.
	if _, again := call(t, "GET", api+"/environments/edge", "static", nil); again["agent_token"] != nil {
		t.Fatal("the agent token must only be returned when issued")
	}
	// Not connected yet: requests explain why.
	if code, out := call(t, "GET", api+"/environments/edge/containers", "static", nil); code != 500 || !strings.Contains(out["error"].(string), "not connected") {
		t.Fatalf("before connect: %d %v", code, out)
	}

	// Wrong or missing tokens are refused before any upgrade.
	if _, err := runAgent(t, ctx, ts.URL, "la_wrong", ""); err == nil {
		t.Fatal("expected a wrong token to be refused")
	}

	name, err := runAgent(t, ctx, ts.URL, token, unixDocker(t))
	if err != nil || name != "edge" {
		t.Fatalf("connect: %v %q", err, name)
	}
	eventually(t, "the agent to show as connected", func() bool {
		_, e := call(t, "GET", api+"/environments/edge", "static", nil)
		return e["agent_connected"] == true
	})
	code, out = call(t, "GET", api+"/environments/edge/containers", "static", nil)
	if code != 200 || out["containers"].([]any)[0].(map[string]any)["Id"] != "behind-nat" {
		t.Fatalf("containers through the agent: %d %v", code, out)
	}

	// Rotating the token drops the agent and locks out the old token.
	code, out = call(t, "PATCH", api+"/environments/edge", "static", map[string]any{"rotate_agent_token": true})
	if code != 200 || out["agent_token"] == token || out["agent_token"] == nil {
		t.Fatalf("rotate: %d %v", code, out)
	}
	eventually(t, "the old agent to be dropped", func() bool {
		_, e := call(t, "GET", api+"/environments/edge", "static", nil)
		return e["agent_connected"] == false
	})
	if _, err := runAgent(t, ctx, ts.URL, token, ""); err == nil {
		t.Fatal("the old token should no longer connect")
	}
	if _, err := runAgent(t, ctx, ts.URL, out["agent_token"].(string), unixDocker(t)); err != nil {
		t.Fatalf("new token: %v", err)
	}

	// Rotating isn't possible for other kinds.
	if code, _ := call(t, "PATCH", api+"/environments/local", "static", map[string]any{"rotate_agent_token": true}); code != 400 {
		t.Fatalf("rotate on local: got %d", code)
	}

	// Audit shows the agent coming and going.
	_, audit := call(t, "GET", api+"/audit", "static", nil)
	seen := map[string]bool{}
	for _, a := range audit["audit"].([]any) {
		m := a.(map[string]any)
		if m["username"] == "agent:edge" {
			seen[m["method"].(string)] = true
		}
	}
	if !seen["CONNECT"] {
		t.Fatalf("expected CONNECT in the audit log: %v", seen)
	}
}
