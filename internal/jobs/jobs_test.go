package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

// fakeHost is a Docker API that records which containers were started.
type fakeHost struct {
	mu      sync.Mutex
	started []string
	svc     *docker.Service
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	f := &fakeHost{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/start"):
			parts := strings.Split(r.URL.Path, "/")
			f.mu.Lock()
			f.started = append(f.started, parts[len(parts)-2])
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	svc, err := docker.New("tcp://"+srv.Listener.Addr().String(), "example.test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

func TestJobsPickTheirHost(t *testing.T) {
	def, edge := newFakeHost(t), newFakeHost(t)
	c := &Consumer{Timeout: 5 * time.Second, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolve: func(host string) (*docker.Service, error) {
			switch host {
			case "":
				return def.svc, nil
			case "edge":
				return edge.svc, nil
			}
			return nil, errors.New("not found")
		}}
	ctx := context.Background()

	if err := c.Process(ctx, Job{ID: "1", Action: "start", Target: "web"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Process(ctx, Job{ID: "2", Action: "start", Target: "api", Host: "edge"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Process(ctx, Job{ID: "3", Action: "start", Target: "x", Host: "nope"}); err == nil {
		t.Fatal("expected an unknown host to fail")
	}
	if len(def.started) != 1 || def.started[0] != "web" || len(edge.started) != 1 || edge.started[0] != "api" {
		t.Fatalf("default started %v, edge started %v", def.started, edge.started)
	}

	// Without a resolver the single Docker host is used, and naming a host is an error.
	single := &Consumer{Timeout: 5 * time.Second, Docker: def.svc, Log: c.Log}
	if err := single.Process(ctx, Job{ID: "4", Action: "start", Target: "db"}); err != nil {
		t.Fatal(err)
	}
	if err := single.Process(ctx, Job{ID: "5", Action: "start", Target: "db", Host: "edge"}); err == nil {
		t.Fatal("expected host on a single-host consumer to fail")
	}
}

// Existing senders put their own stage name in "environment"; that must not
// be read as a host.
func TestEnvironmentFieldIsNotAHost(t *testing.T) {
	var m Message
	body := `{"projectId":"p","deployConfig":{"id":"1","action":"start","target":"web","environment":"production"}}`
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if m.DeployConfig.Host != "" || m.DeployConfig.Environment != "production" {
		t.Fatalf("unexpected decode: %+v", m.DeployConfig)
	}
}
