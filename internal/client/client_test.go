package client

import "testing"

func TestScope(t *testing.T) {
	c := &Client{}
	if got := c.Scope("/api/v1/containers"); got != "/api/v1/containers" {
		t.Fatalf("no env should not rewrite: %s", got)
	}
	c.Env = "edge"
	cases := map[string]string{
		"/api/v1/containers":            "/api/v1/environments/edge/containers",
		"/api/v1/containers/abc/logs?f": "/api/v1/environments/edge/containers/abc/logs?f",
		"/api/v1/images?all=1":          "/api/v1/environments/edge/images?all=1",
		"/api/v1/system/info":           "/api/v1/environments/edge/system/info",
		"/api/v1/auth/me":               "/api/v1/auth/me",
		"/api/v1/users":                 "/api/v1/users",
		"/api/v1/environments":          "/api/v1/environments",
		"/api/v1/containersx":           "/api/v1/containersx",
	}
	for in, want := range cases {
		if got := c.Scope(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}
