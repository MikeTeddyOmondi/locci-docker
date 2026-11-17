package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// fakeDocker answers /_ping and lists one container.
func fakeDocker(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Id":"abc123","Names":["/web"],"Image":"nginx","State":"running"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return "tcp://" + srv.Listener.Addr().String()
}

func TestEnvironmentRouting(t *testing.T) {
	ts, db := newTestServer(t) // default "local" is down
	if _, err := db.CreateEnvironment(store.Environment{Name: "fake", Kind: store.KindLocal, URL: fakeDocker(t)}); err != nil {
		t.Fatal(err)
	}
	api := ts.URL + "/api/v1"

	// The original routes keep using the default environment.
	if code, _ := call(t, "GET", api+"/containers", "static", nil); code != 500 {
		t.Fatalf("default env containers: got %d", code)
	}
	if code, _ := call(t, "GET", api+"/environments/local/containers", "static", nil); code != 500 {
		t.Fatalf("explicit local: got %d", code)
	}

	// A scoped route reaches the chosen environment.
	code, out := call(t, "GET", api+"/environments/fake/containers", "static", nil)
	if code != 200 {
		t.Fatalf("fake containers: %d %v", code, out)
	}
	list := out["containers"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["Id"] != "abc123" {
		t.Fatalf("unexpected list: %v", out)
	}

	// Unknown environments are a 404, still behind auth.
	if code, out := call(t, "GET", api+"/environments/nope/containers", "static", nil); code != 404 || !strings.Contains(out["error"].(string), "nope") {
		t.Fatalf("unknown env: %d %v", code, out)
	}
	if code, _ := call(t, "GET", api+"/environments/fake/containers", "", nil); code != 401 {
		t.Fatalf("no token: got %d", code)
	}

	// Legacy-only routes exist on the default env, not under /environments/:env.
	if code, _ := call(t, "POST", api+"/environments/fake/containers/start", "static", map[string]string{"container_id": "x"}); code != 404 {
		t.Fatalf("legacy route should not be scoped: got %d", code)
	}

	// Moving the default moves the original routes with it.
	if err := db.SetDefaultEnvironment("fake"); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, "GET", api+"/containers", "static", nil); code != 200 {
		t.Fatalf("containers after default moved: got %d", code)
	}
	if code, _ := call(t, "GET", ts.URL+"/health", "", nil); code != 200 {
		t.Fatalf("health after default moved: got %d", code)
	}
}
