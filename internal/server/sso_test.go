package server

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/sso/ssotest"
)

// browser follows redirects like a browser, with cookies, but stops at the
// final landing URL (the web /login page or the CLI's loopback listener).
func browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Path == "/login" || req.URL.Path == "/callback" {
			return http.ErrUseLastResponse
		}
		return nil
	}}
}

// signIn runs the whole flow and returns where it landed.
func signIn(t *testing.T, startURL string) *url.URL {
	u, _ := signInSession(t, startURL)
	return u
}

// signInSession is signIn that also returns the session cookie the last
// response set ("" if none).
func signInSession(t *testing.T, startURL string) (*url.URL, string) {
	t.Helper()
	resp, err := browser().Get(startURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected a redirect at the end, got %d", resp.StatusCode)
	}
	u, _ := url.Parse(resp.Header.Get("Location"))
	session := ""
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie {
			session = ck.Value
			if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
				t.Errorf("session cookie flags: %+v", ck)
			}
		}
	}
	return u, session
}

func TestSSOFlow(t *testing.T) {
	ts, db := newTestServerWithKey(t, testBox(t))
	api := ts.URL + "/api/v1"
	idp := ssotest.New(t)

	// Nothing configured: password only, OIDC routes don't exist.
	code, out := call(t, "GET", api+"/auth/providers", "", nil)
	if code != 200 || out["password_login"] != true || len(out["providers"].([]any)) != 0 {
		t.Fatalf("providers before setup: %d %v", code, out)
	}
	if code, _ := call(t, "GET", api+"/auth/oidc/fake/start", "", nil); code != 404 {
		t.Fatalf("start before setup: got %d", code)
	}

	// Admin adds a provider; it starts disabled and never shows its secret.
	db.CreateUser("ann", "password123", "user")
	_, out = call(t, "POST", api+"/auth/login", "", map[string]string{"username": "ann", "password": "password123"})
	ann := out["token"].(string)
	body := map[string]any{"slug": "fake", "display_name": "Fake IdP", "issuer_url": idp.URL, "client_id": idp.ClientID,
		"client_secret": idp.ClientSecret, "auto_create_users": true, "admin_group": "ops"}
	if code, _ := call(t, "POST", api+"/sso/providers", ann, body); code != 403 {
		t.Fatalf("create as user: got %d", code)
	}
	code, out = call(t, "POST", api+"/sso/providers", "static", body)
	if code != 201 || out["has_secret"] != true || out["client_secret"] != nil || out["enabled"] != false {
		t.Fatalf("create: %d %v", code, out)
	}
	if !strings.HasSuffix(out["redirect_uri"].(string), "/api/v1/auth/oidc/fake/callback") {
		t.Fatalf("redirect uri: %v", out["redirect_uri"])
	}
	if code, _ := call(t, "GET", api+"/auth/oidc/fake/start", "", nil); code != 404 {
		t.Fatalf("disabled provider should 404, got %d", code)
	}

	// Discovery test.
	if code, out := call(t, "POST", api+"/sso/providers/test", "static", map[string]string{"issuer_url": idp.URL}); code != 200 || out["ok"] != true {
		t.Fatalf("discovery: %d %v", code, out)
	}
	if code, out := call(t, "POST", api+"/sso/providers/test", "static", map[string]string{"issuer_url": "http://127.0.0.1:1"}); code != 200 || out["ok"] != false {
		t.Fatalf("discovery of nothing: %d %v", code, out)
	}

	// Enable it: the login page lists it.
	if code, _ := call(t, "PATCH", api+"/sso/providers/fake", "static", map[string]any{"enabled": true}); code != 200 {
		t.Fatalf("enable: got %d", code)
	}
	_, out = call(t, "GET", api+"/auth/providers", "", nil)
	if list := out["providers"].([]any); len(list) != 1 || list[0].(map[string]any)["display_name"] != "Fake IdP" {
		t.Fatalf("enabled list: %v", out)
	}

	// Browser sign-in: the token arrives as the session cookie, and the
	// fragment on /login only says it worked.
	idp.SetUser(map[string]any{"sub": "u-7", "email": "kim@example.com", "email_verified": true, "groups": []string{"ops"}})
	land, session := signInSession(t, api+"/auth/oidc/fake/start")
	frag, _ := url.ParseQuery(land.Fragment)
	if land.Path != "/login" || frag.Get("signed_in") != "1" || frag.Get("token") != "" || session == "" || land.RawQuery != "" {
		t.Fatalf("landing: %s (session cookie %t)", land, session != "")
	}
	code, me := call(t, "GET", api+"/auth/me", session, nil)
	if code != 200 || me["username"] != "kim@example.com" || me["role"] != "admin" || me["has_password"] != false {
		t.Fatalf("me: %d %v", code, me)
	}
	_, out = call(t, "GET", api+"/audit", "static", nil)
	if a := out["audit"].([]any)[0].(map[string]any); a["method"] != "LOGIN" || a["username"] != "kim@example.com" || a["status"] != 200.0 {
		t.Fatalf("audit: %v", a)
	}

	// CLI sign-in: lands on the loopback listener with the token in the query.
	land = signIn(t, api+"/auth/oidc/fake/start?cli_port=45678")
	if land.Host != "127.0.0.1:45678" || land.Path != "/callback" || land.Query().Get("token") == "" {
		t.Fatalf("cli landing: %s", land)
	}

	// The provider refusing: back to /login with an error, and audited.
	idp.Deny()
	land = signIn(t, api+"/auth/oidc/fake/start")
	if frag, _ := url.ParseQuery(land.Fragment); frag.Get("error") == "" || frag.Get("token") != "" {
		t.Fatalf("denied landing: %s", land)
	}

	// A callback without the flow cookie is refused.
	resp, _ := browser().Get(api + "/auth/oidc/fake/callback?code=x&state=y")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "error=") {
		t.Fatalf("callback without cookie: %s", loc)
	}

	// Turning password login off: passwords are refused, the last provider
	// can't be disabled, and it can be turned back on.
	if code, _ := call(t, "PUT", api+"/sso/settings", "static", map[string]any{"password_login": false}); code != 200 {
		t.Fatalf("password login off: got %d", code)
	}
	if code, out := call(t, "POST", api+"/auth/login", "", map[string]string{"username": "ann", "password": "password123"}); code != 403 {
		t.Fatalf("password login while off: %d %v", code, out)
	}
	if code, _ := call(t, "PATCH", api+"/sso/providers/fake", "static", map[string]any{"enabled": false}); code != 409 {
		t.Fatalf("disabling the last provider: got %d", code)
	}
	if code, _ := call(t, "DELETE", api+"/sso/providers/fake", "static", nil); code != 409 {
		t.Fatalf("deleting the last provider: got %d", code)
	}
	if code, _ := call(t, "PUT", api+"/sso/settings", "static", map[string]any{"password_login": true}); code != 200 {
		t.Fatalf("password login on: got %d", code)
	}
	if code, _ := call(t, "DELETE", api+"/sso/providers/fake", "static", nil); code != 200 {
		t.Fatalf("delete: got %d", code)
	}
}

func TestSSONeedsSecretKey(t *testing.T) {
	ts, _ := newTestServer(t) // no LOCCI_SECRET_KEY
	body := map[string]any{"slug": "kc", "issuer_url": "https://id.example.com", "client_id": "c"}
	if code, out := call(t, "POST", ts.URL+"/api/v1/sso/providers", "static", body); code != 412 {
		t.Fatalf("expected 412, got %d %v", code, out)
	}
	// Password login can't be turned off with no provider.
	if code, _ := call(t, "PUT", ts.URL+"/api/v1/sso/settings", "static", map[string]any{"password_login": false}); code != 409 {
		t.Fatalf("expected 409, got %d", code)
	}
}

func TestSSODeviceFlow(t *testing.T) {
	ts, _ := newTestServerWithKey(t, testBox(t))
	api := ts.URL + "/api/v1"
	idp := ssotest.New(t)

	// Disabled or unknown providers don't offer it.
	if code, _ := call(t, "POST", api+"/auth/oidc/fake/device", "", nil); code != 404 {
		t.Fatalf("device before setup: got %d", code)
	}
	call(t, "POST", api+"/sso/providers", "static", map[string]any{"slug": "fake", "issuer_url": idp.URL,
		"client_id": idp.ClientID, "client_secret": idp.ClientSecret, "auto_create_users": true, "enabled": true})

	code, out := call(t, "POST", api+"/auth/oidc/fake/device", "", nil)
	if code != 200 || out["user_code"] != "ABCD-EFGH" || out["handle"] == "" || out["device_code"] != nil {
		t.Fatalf("device start: %d %v", code, out)
	}
	handle := out["handle"].(string)

	if code, out := call(t, "POST", api+"/auth/oidc/fake/device/token", "", map[string]string{"handle": handle}); code != 200 || out["status"] != "pending" {
		t.Fatalf("pending: %d %v", code, out)
	}
	idp.SetUser(map[string]any{"sub": "d-9", "email": "ops@example.com", "email_verified": true})
	idp.ApproveDevice()
	code, out = call(t, "POST", api+"/auth/oidc/fake/device/token", "", map[string]string{"handle": handle})
	if code != 200 || out["status"] != "ok" || out["token"] == "" {
		t.Fatalf("approved: %d %v", code, out)
	}
	if code, me := call(t, "GET", api+"/auth/me", out["token"].(string), nil); code != 200 || me["username"] != "ops@example.com" {
		t.Fatalf("me: %d %v", code, me)
	}
	_, audit := call(t, "GET", api+"/audit", "static", nil)
	if a := audit["audit"].([]any)[0].(map[string]any); a["method"] != "LOGIN" || a["username"] != "ops@example.com" {
		t.Fatalf("audit: %v", a)
	}

	// A garbage handle is refused.
	if code, _ := call(t, "POST", api+"/auth/oidc/fake/device/token", "", map[string]string{"handle": "nope"}); code != 400 {
		t.Fatalf("bad handle: got %d", code)
	}
}
