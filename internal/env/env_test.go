package env

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

func newRegistry(t *testing.T, box *secret.Box) *Registry {
	t.Helper()
	db, err := store.Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &Registry{Store: db, Box: box, Domain: "example.test", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	t.Cleanup(r.Close)
	return r
}

// fakeDocker is a plain-HTTP server that answers /_ping.
func fakeDocker(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("API-Version", "1.47")
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return "tcp://" + srv.Listener.Addr().String()
}

func TestRegistry(t *testing.T) {
	r := newRegistry(t, nil)
	ctx := context.Background()

	// "local" with a URL pointing at the fake daemon; "dead" points nowhere.
	up, err := r.Store.CreateEnvironment(store.Environment{Name: "local", Kind: store.KindLocal, URL: fakeDocker(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Store.CreateEnvironment(store.Environment{Name: "dead", Kind: store.KindLocal, URL: "tcp://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}

	// Lazy connect and caching: the same name gives the same client.
	a, e, err := r.Get("local")
	if err != nil || e.ID != up.ID {
		t.Fatal(err)
	}
	b, _, _ := r.Get("local")
	if a != b {
		t.Fatal("expected the cached service to be reused")
	}
	if d, de, _ := r.Default(); d != a || de.Name != "local" {
		t.Fatal("default should be the first environment")
	}

	// Health: statuses are recorded.
	r.CheckAll(ctx)
	if e, _ := r.Store.EnvironmentByName("local"); e.Status != store.StatusUp || e.LastSeenAt == "" {
		t.Fatalf("local: %+v", e)
	}
	if e, _ := r.Store.EnvironmentByName("dead"); e.Status != store.StatusDown {
		t.Fatalf("dead: %+v", e)
	}

	// Editing the connection settings rebuilds the client on the next Get.
	up.URL = fakeDocker(t)
	if _, err := r.Store.UpdateEnvironment(up); err != nil {
		t.Fatal(err)
	}
	c, _, _ := r.Get("local")
	if c == a {
		t.Fatal("expected a new client after the URL changed")
	}
	// A label-only edit keeps the client.
	up.Label = "Laptop"
	r.Store.UpdateEnvironment(up)
	if d, _, _ := r.Get("local"); d != c {
		t.Fatal("label change should not rebuild the client")
	}

	if _, _, err := r.Get("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Test() talks to an unsaved host and caches nothing.
	o, _ := r.Options(store.Environment{Kind: store.KindLocal, URL: fakeDocker(t)})
	if _, err := Test(ctx, o); err != nil {
		t.Fatalf("test up: %v", err)
	}
	o.Host = "tcp://127.0.0.1:1"
	if _, err := Test(ctx, o); err == nil {
		t.Fatal("expected test against a closed port to fail")
	}
}

func TestSecretsNeedKey(t *testing.T) {
	// Without LOCCI_SECRET_KEY a remote environment can't be opened.
	r := newRegistry(t, nil)
	e := store.Environment{Name: "edge", Kind: store.KindTLS, URL: "tcp://10.0.0.5:2376", Secret: "v1:abc"}
	if _, err := r.Options(e); !errors.Is(err, secret.ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}

	// With a key the secret is decrypted into the TLS options.
	k := make([]byte, 32)
	rand.Read(k)
	box, _ := secret.New(base64.StdEncoding.EncodeToString(k))
	sealed, _ := box.Seal("KEY-PEM")
	r = newRegistry(t, box)
	e.Secret, e.TLSCA, e.TLSCert = sealed, "CA", "CERT"
	o, err := r.Options(e)
	if err != nil || o.TLS == nil || o.TLS.Key != "KEY-PEM" || o.TLS.CA != "CA" {
		t.Fatalf("options: %v %+v", err, o.TLS)
	}
}
