package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/ui"
)

// Client-side routes are served index.html with 200; unknown API paths stay
// JSON 404s.
func TestUIRoutes(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, path := range []string{"/", "/login", "/e/local/containers", "/environments"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), "<") {
			t.Errorf("%s: got %d", path, resp.StatusCode)
		}
	}
	if code, out := call(t, "GET", ts.URL+"/api/v1/nope", "static", nil); code != 404 || out["error"] != "not found" {
		t.Errorf("unknown API path: %d %v", code, out)
	}
	if !ui.Built() {
		return // CI's Go job doesn't build the web UI; static files come with it
	}
	resp, _ := http.Get(ts.URL + "/favicon.svg")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("static file: got %d", resp.StatusCode)
	}
}
