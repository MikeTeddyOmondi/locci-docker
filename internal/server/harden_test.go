package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, path := range []string{"/login", "/health", "/api/v1/auth/me"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		h := resp.Header
		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "script-src 'self'") {
			t.Errorf("%s: CSP %q", path, csp)
		}
		if h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers %v", path, h)
		}
		if h.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS over plain http", path)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	big := bytes.Repeat([]byte("a"), MaxBody+1)
	body := append(append([]byte(`{"username":"x","password":"`), big...), '"', '}')
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("got %d, want 413", resp.StatusCode)
	}
}
