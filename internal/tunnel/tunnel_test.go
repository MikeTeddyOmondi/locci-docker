package tunnel

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
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeDocker serves a tiny Docker API on a unix socket and returns its path.
func fakeDocker(t *testing.T, name string) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "tun")
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
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Name":"` + name + `"}`))
		default:
			http.NotFound(w, r)
		}
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

// server upgrades /connect into a tunnel for environment 1.
func server(t *testing.T, hub *Hub) string {
	up := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.Attach(1, NewConn(ws), nil)
	}))
	t.Cleanup(ts.Close)
	return "ws" + strings.TrimPrefix(ts.URL, "http")
}

// agent connects to the server and serves sock until ctx ends.
func agent(t *testing.T, ctx context.Context, url, sock string) {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	go Serve(ctx, NewConn(ws), func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}, quiet)
}

func dockerVia(t *testing.T, hub *Hub) *client.Client {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.WithHost("http://docker.invalid"), client.WithAPIVersionNegotiation(),
		client.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) { return hub.Dial(ctx, 1) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	return cli
}

func waitConnected(t *testing.T, hub *Hub, want bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if hub.Connected(1) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("connected never became %v", want)
}

func TestTunnel(t *testing.T) {
	hub := &Hub{}
	url := server(t, hub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := hub.Dial(ctx, 1); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected before an agent attaches, got %v", err)
	}

	agent(t, ctx, url, fakeDocker(t, "first"))
	waitConnected(t, hub, true)
	cli := dockerVia(t, hub)
	if _, err := cli.Ping(ctx); err != nil {
		t.Fatalf("ping through the tunnel: %v", err)
	}
	info, err := cli.Info(ctx)
	if err != nil || info.Name != "first" {
		t.Fatalf("info through the tunnel: %v %+v", err, info.Name)
	}

	// Many requests share the one WebSocket.
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cli.Info(ctx)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel request: %v", err)
		}
	}

	// A second agent for the same environment replaces the first.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	agent(t, ctx2, url, fakeDocker(t, "second"))
	for i := 0; i < 100; i++ {
		cli.Close()
		cli = dockerVia(t, hub)
		if info, err := cli.Info(ctx); err == nil && info.Name == "second" {
			break
		}
		time.Sleep(20 * time.Millisecond)
		if i == 99 {
			t.Fatal("the new agent never took over")
		}
	}

	// Disconnect closes it; dialing fails until an agent comes back.
	hub.Disconnect(1)
	waitConnected(t, hub, false)
	if _, err := hub.Dial(ctx, 1); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected after Disconnect, got %v", err)
	}
}
