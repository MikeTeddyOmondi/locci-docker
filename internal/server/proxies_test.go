package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseProxies(t *testing.T) {
	for in, want := range map[string]int{"": 7, "none": 0, "*": 2, "10.1.2.3, fd00::1, 192.168.0.0/16": 3} {
		got, err := ParseProxies(in)
		if err != nil || len(got) != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseProxies("10.0.0.0/8,nope"); err == nil {
		t.Error("bad entry accepted")
	}
}

// X-Forwarded-* decide the OIDC redirect URI and Secure cookies, so they count
// only from a trusted proxy.
func TestForwardedHeadersNeedTrustedProxy(t *testing.T) {
	ctx := func(remote string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "http://locci.internal/x", nil)
		c.Request.RemoteAddr = remote
		c.Request.Header.Set("X-Forwarded-Proto", "https")
		c.Request.Header.Set("X-Forwarded-Host", "evil.example")
		return c
	}
	def := &Server{} // DefaultProxies
	if got := def.baseURL(ctx("172.18.0.5:4000")); got != "https://evil.example" {
		t.Errorf("proxy on a Docker network: %s", got)
	}
	if got := def.baseURL(ctx("203.0.113.9:4000")); got != "http://locci.internal" {
		t.Errorf("public client: %s", got)
	}
	none := &Server{TrustedProxies: []string{}}
	if got := none.baseURL(ctx("127.0.0.1:4000")); got != "http://locci.internal" {
		t.Errorf("no proxies trusted: %s", got)
	}
}

func TestQueryTokenOnlyForWebSockets(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/auth/me?token=static")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("?token= on a plain request: got %d", resp.StatusCode)
	}
	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me", "static", nil); code != 200 {
		t.Errorf("header token: got %d", code)
	}
}

func TestSessionsAPI(t *testing.T) {
	ts, db := newTestServer(t)
	db.CreateUser("ann", "correct-horse", "user")
	login := func() string {
		_, out := call(t, "POST", ts.URL+"/api/v1/auth/login", "", map[string]string{"username": "ann", "password": "correct-horse"})
		return out["token"].(string)
	}
	laptop, phone := login(), login()

	_, out := call(t, "GET", ts.URL+"/api/v1/auth/tokens", laptop, nil)
	list, _ := out["tokens"].([]any)
	if len(list) != 2 {
		t.Fatalf("tokens: %v", out)
	}
	var other string
	for _, x := range list {
		m := x.(map[string]any)
		if m["current"] != true {
			other = m["id"].(string)
		}
	}
	if code, _ := call(t, "DELETE", ts.URL+"/api/v1/auth/tokens/"+other, laptop, nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := call(t, "GET", ts.URL+"/api/v1/auth/me", phone, nil); code != 401 {
		t.Fatalf("revoked session still works: %d", code)
	}
	if code, _ := call(t, "DELETE", ts.URL+"/api/v1/auth/tokens/"+other, laptop, nil); code != 404 {
		t.Fatalf("revoke twice: %d", code)
	}
}
