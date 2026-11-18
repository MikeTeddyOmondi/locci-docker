package server

import (
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

func TestEnvironmentAccess(t *testing.T) {
	ts, db := newTestServer(t)
	api := ts.URL + "/api/v1"
	db.CreateUser("ann", "correct-horse", "user")
	db.CreateUser("bob", "correct-horse", "user")
	db.CreateEnvironment(store.Environment{Name: "prod", Kind: store.KindLocal, URL: "tcp://127.0.0.1:1"})
	login := func(name string) string {
		_, out := call(t, "POST", api+"/auth/login", "", map[string]string{"username": name, "password": "correct-horse"})
		return out["token"].(string)
	}
	ann, bob := login("ann"), login("bob")
	names := func(tok string) []string {
		_, out := call(t, "GET", api+"/environments", tok, nil)
		var n []string
		for _, e := range out["environments"].([]any) {
			n = append(n, e.(map[string]any)["name"].(string))
		}
		return n
	}
	set := func(tok, env string, body map[string]any) (int, map[string]any) {
		return call(t, "PUT", api+"/environments/"+env+"/access", tok, body)
	}

	// Open by default: everyone sees and uses every environment.
	if got := names(bob); len(got) != 2 {
		t.Fatalf("open environments: %v", got)
	}

	// Only admins set access; unknown users are refused.
	if code, _ := set(ann, "prod", map[string]any{"restricted": true, "users": []string{"ann"}}); code != 403 {
		t.Fatalf("set as user: %d", code)
	}
	if code, _ := set("static", "prod", map[string]any{"restricted": true, "users": []string{"nobody"}}); code != 404 {
		t.Fatalf("unknown user: %d", code)
	}
	code, out := set("static", "prod", map[string]any{"restricted": true, "users": []string{"ann", "ann", ""}})
	if code != 200 || out["restricted"] != true || len(out["users"].([]any)) != 1 {
		t.Fatalf("restrict: %d %v", code, out)
	}

	// Ann keeps prod; Bob no longer sees or reaches it (404, as if absent).
	if got := names(ann); len(got) != 2 {
		t.Errorf("ann: %v", got)
	}
	if got := names(bob); len(got) != 1 || got[0] != "local" {
		t.Errorf("bob: %v", got)
	}
	for _, path := range []string{"/environments/prod", "/environments/prod/containers", "/environments/prod/info"} {
		if code, _ := call(t, "GET", api+path, bob, nil); code != 404 {
			t.Errorf("bob %s: %d", path, code)
		}
	}
	// Ann is let through (Docker is down in tests: anything but 403/404).
	if code, _ := call(t, "GET", api+"/environments/prod/containers", ann, nil); code == 403 || code == 404 {
		t.Errorf("ann containers: %d", code)
	}
	// Admins see the settings; plain users don't.
	_, out = call(t, "GET", api+"/environments/prod", "static", nil)
	if a, _ := out["access"].(map[string]any); a["restricted"] != true || len(a["users"].([]any)) != 1 {
		t.Errorf("admin view: %v", out["access"])
	}
	if _, out = call(t, "GET", api+"/environments/prod", ann, nil); out["access"] != nil {
		t.Errorf("user sees access settings: %v", out["access"])
	}

	// A restricted default environment blocks the original routes for others.
	set("static", "local", map[string]any{"restricted": true, "users": []string{"ann"}})
	if code, _ := call(t, "GET", api+"/containers", bob, nil); code != 403 {
		t.Errorf("bob on a restricted default: %d", code)
	}

	// Deleting the only listed user leaves the environment admin-only; it
	// doesn't open up.
	if err := db.DeleteUser("ann"); err != nil {
		t.Fatal(err)
	}
	if got := names(bob); len(got) != 0 {
		t.Errorf("bob after ann was deleted: %v", got)
	}

	// Opening it again takes an explicit restricted=false.
	if code, out := set("static", "prod", map[string]any{"restricted": false}); code != 200 || out["restricted"] != false {
		t.Fatalf("open: %d %v", code, out)
	}
	if got := names(bob); len(got) != 1 || got[0] != "prod" {
		t.Errorf("bob after reopening prod: %v", got)
	}
}
