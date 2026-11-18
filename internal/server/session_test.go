package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// cookieCall sends a request with the session cookie and, if csrf, the CSRF header.
func cookieCall(t *testing.T, method, url, session string, csrf bool, body any) (int, map[string]any, *http.Response) {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
	}
	if csrf {
		req.Header.Set(csrfHeader, "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp
}

func sessionFrom(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

func TestCookieSession(t *testing.T) {
	ts, db := newTestServer(t)
	db.CreateUser("ann", "correct-horse", "user")
	api := ts.URL + "/api/v1"

	// The web UI's login: token in an HttpOnly cookie, not in the body.
	code, out, resp := cookieCall(t, "POST", api+"/auth/login", "", true, map[string]any{"username": "ann", "password": "correct-horse", "cookie": true})
	ck := sessionFrom(resp)
	if code != 200 || out["token"] != nil || ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Fatalf("cookie login: %d %v %+v", code, out, ck)
	}
	session := ck.Value

	if code, out, _ := cookieCall(t, "GET", api+"/auth/me", session, false, nil); code != 200 || out["username"] != "ann" {
		t.Fatalf("me with the cookie: %d %v", code, out)
	}
	// Changes need the CSRF header with a cookie...
	if code, out, _ := cookieCall(t, "POST", api+"/auth/logout", session, false, nil); code != 403 {
		t.Fatalf("logout without CSRF header: %d %v", code, out)
	}
	// ...but not with a bearer token (the CLI).
	if code, _ := call(t, "GET", api+"/auth/tokens", session, nil); code != 200 {
		t.Fatalf("bearer still works: %d", code)
	}
	code, _, resp = cookieCall(t, "POST", api+"/auth/logout", session, true, nil)
	if c := sessionFrom(resp); code != 200 || c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout: %d, cookie %+v", code, c)
	}
	// A dead session clears the cookie.
	code, _, resp = cookieCall(t, "GET", api+"/auth/me", session, false, nil)
	if c := sessionFrom(resp); code != 401 || c == nil || c.MaxAge >= 0 {
		t.Fatalf("after logout: %d, cookie %+v", code, c)
	}

	// A token from local storage (before cookies) moves into the cookie.
	_, out = call(t, "POST", api+"/auth/login", "", map[string]string{"username": "ann", "password": "correct-horse"})
	old := out["token"].(string)
	code, _, resp = cookieCall(t, "POST", api+"/auth/session", "", true, nil)
	if code != 401 {
		t.Fatalf("adopt without a token: %d", code)
	}
	req, _ := http.NewRequest("POST", api+"/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+old)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if c := sessionFrom(resp); resp.StatusCode != 200 || c == nil || c.Value != old {
		t.Fatalf("adopt: %d %+v", resp.StatusCode, c)
	}
	// The static AUTH_TOKEN never becomes a browser session.
	if code, _ := call(t, "POST", api+"/auth/session", "static", nil); code != 400 {
		t.Fatalf("adopt AUTH_TOKEN: %d", code)
	}
}

func TestWebSocketOrigin(t *testing.T) {
	req := func(host, origin, remote, fwdHost string) *http.Request {
		r := httptest.NewRequest("GET", "http://"+host+"/x", nil)
		r.Host = host
		r.RemoteAddr = remote
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if fwdHost != "" {
			r.Header.Set("X-Forwarded-Host", fwdHost)
		}
		return r
	}
	s := &Server{}
	pub := &Server{PublicURL: "https://locci.example.com"}
	cases := []struct {
		name string
		s    *Server
		r    *http.Request
		want bool
	}{
		{"no Origin (CLI, agent)", s, req("locci:8998", "", "203.0.113.1:1", ""), true},
		{"same host", s, req("locci.example.com", "https://locci.example.com", "203.0.113.1:1", ""), true},
		{"other site", s, req("locci.example.com", "https://evil.example", "203.0.113.1:1", ""), false},
		{"suffix trick", s, req("example.com", "https://evilexample.com", "203.0.113.1:1", ""), false},
		{"proxy rewrote Host, trusted X-Forwarded-Host", s, req("locci:8998", "https://locci.example.com", "172.18.0.2:1", "locci.example.com"), true},
		{"X-Forwarded-Host from an untrusted client", s, req("locci:8998", "https://evil.example", "203.0.113.1:1", "evil.example"), false},
		{"LOCCI_PUBLIC_URL", pub, req("locci:8998", "https://locci.example.com", "203.0.113.1:1", ""), true},
	}
	for _, c := range cases {
		if got := c.s.sameOrigin(c.r); got != c.want {
			t.Errorf("%s: got %t", c.name, got)
		}
	}
}
