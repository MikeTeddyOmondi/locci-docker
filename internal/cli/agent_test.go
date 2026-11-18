package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeTeddyOmondi/locci-docker/internal/client"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/server"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/tunnel"
)

func TestAgentURLAndDockerHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://locci.example.com/": "wss://locci.example.com/api/v1/agent/connect",
		"http://127.0.0.1:8999":      "ws://127.0.0.1:8999/api/v1/agent/connect",
		"https://example.com/locci":  "wss://example.com/locci/api/v1/agent/connect",
	} {
		if got, err := agentURL(in); err != nil || got != want {
			t.Errorf("%s: %v %s", in, err, got)
		}
	}
	if _, err := agentURL("ftp://x"); err == nil {
		t.Error("expected a non-http server URL to be refused")
	}
	if _, err := dockerDialer("npipe:////./pipe/docker_engine"); err == nil {
		t.Error("expected an unsupported docker host to be refused")
	}
	if _, err := dockerDialer("tcp://127.0.0.1:2375"); err != nil {
		t.Error(err)
	}
}

func TestAgentAgainstServer(t *testing.T) {
	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db.EnsureDefaultEnvironment()
	e, _ := db.CreateEnvironment(store.Environment{Name: "nat-box", Kind: store.KindAgent})
	token, _ := db.NewAgentToken(e.ID)

	srv := &server.Server{Envs: &env.Registry{Store: db, Hub: &tunnel.Hub{}, Log: log}, Store: db, AuthToken: "static", Log: log}
	ts := httptest.NewServer(srv.Router("test"))
	defer ts.Close()

	// The host's Docker, on a unix socket.
	dir, _ := os.MkdirTemp("", "ag")
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "d.sock")
	ln, _ := net.Listen("unix", sock)
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("API-Version", "1.47")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Name":"nat-box-host","ServerVersion":"28.0.0"}`))
	}))
	dial, _ := dockerDialer("unix://" + sock)
	wsURL, _ := agentURL(ts.URL)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runAgent(ctx, wsURL, token, dial, log) }()

	c := client.New(client.Config{URL: ts.URL, Token: "static", Env: "nat-box"})
	var info struct{ Name string }
	for i := 0; ; i++ {
		if err := c.Do("GET", "/api/v1/system/info", nil, &info); err == nil {
			break
		}
		if i > 100 {
			t.Fatal("never reached the host through the agent")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info.Name != "nat-box-host" {
		t.Fatalf("info: %+v", info)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("agent should stop cleanly on cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent didn't stop")
	}

	// A rejected token stops the agent instead of retrying forever.
	err = runAgent(context.Background(), wsURL, "la_nope", dial, log)
	if !errors.Is(err, errBadToken) {
		t.Fatalf("expected errBadToken, got %v", err)
	}
}
