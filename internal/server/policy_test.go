package server

import (
	"testing"
)

// The spec checks run before Docker is asked anything, so they work with the
// test server's unreachable Docker.
func TestPlainUserContainerLimits(t *testing.T) {
	ts, db := newTestServer(t)
	db.CreateUser("ann", "correct-horse", "user")
	_, out := call(t, "POST", ts.URL+"/api/v1/auth/login", "", map[string]string{"username": "ann", "password": "correct-horse"})
	ann := out["token"].(string)

	create := func(tok string, spec map[string]any) (int, string) {
		spec["name"], spec["image"] = "x", "alpine"
		if spec["subdomain"] == nil {
			spec["subdomain"] = "x"
		}
		code, out := call(t, "POST", ts.URL+"/api/v1/containers", tok, spec)
		msg, _ := out["error"].(string)
		return code, msg
	}
	cases := map[string]map[string]any{
		"host path":     {"volumes": []string{"/:/host"}},
		"docker socket": {"volumes": []string{"/var/run/docker.sock:/var/run/docker.sock"}},
		"relative path": {"volumes": []string{"./data:/data"}},
		"traefik label": {"labels": map[string]string{"traefik.http.routers.x.rule": "Host(`bank.example`)"}},
		"locci label":   {"labels": map[string]string{"io.locci.protected": "true"}},
	}
	for name, spec := range cases {
		if code, msg := create(ann, spec); code != 403 {
			t.Errorf("%s as a plain user: %d %s", name, code, msg)
		}
		// Admins (here the static token) aren't limited: Docker is reached
		// and, being down in tests, fails differently.
		if code, _ := create("static", spec); code == 403 {
			t.Errorf("%s as admin: refused", name)
		}
	}
	if code, _ := create(ann, map[string]any{"subdomain": "a.b"}); code != 400 {
		t.Errorf("dotted subdomain: %d", code)
	}
	// A named volume passes the checks (and then needs Docker).
	if code, msg := create(ann, map[string]any{"volumes": []string{"data:/data", "/anon"}}); code == 403 {
		t.Errorf("named volume refused: %s", msg)
	}
}

func TestUserHostAccessLiftsLimits(t *testing.T) {
	// The setting is checked before the user, so no request is needed.
	if (&Server{UserHostAccess: true}).restricted(nil) {
		t.Error("restricted with LOCCI_USER_HOST_ACCESS=true")
	}
}
