package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for range 3 {
		if l.wait("a") != 0 {
			t.Fatal("blocked before the limit")
		}
		l.add("a")
	}
	if d := l.wait("a"); d != time.Minute {
		t.Fatalf("at the limit: wait %v", d)
	}
	if l.wait("b") != 0 {
		t.Fatal("other keys are independent")
	}
	now = now.Add(time.Minute)
	if l.wait("a") != 0 {
		t.Fatal("still blocked after the window")
	}
	l.add("a")
	l.add("a")
	l.add("a")
	l.clear("a")
	if l.wait("a") != 0 {
		t.Fatal("clear didn't reset")
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts, db := newTestServer(t)
	if _, err := db.CreateUser("ann", "correct-horse", "user"); err != nil {
		t.Fatal(err)
	}
	login := func(pw string) int {
		code, _ := call(t, "POST", ts.URL+"/api/v1/auth/login", "", map[string]string{"username": "ann", "password": pw})
		return code
	}
	for i := range 10 {
		if code := login("wrong"); code != 401 {
			t.Fatalf("attempt %d: got %d", i+1, code)
		}
	}
	// The 11th try from this IP is refused even with the right password.
	if code := login("correct-horse"); code != 429 {
		t.Fatalf("after 10 failures: got %d, want 429", code)
	}
	entries, _ := db.ListAudit(50)
	var failed, limited int
	for _, e := range entries {
		switch e.Status {
		case 401:
			failed++
		case 429:
			limited++
		}
	}
	if failed != 10 || limited != 1 {
		t.Errorf("audit: %d failed, %d limited", failed, limited)
	}
}

// Guessing a user's password from many addresses locks the username, but not
// for the address the user signed in from before.
func TestUsernameLockoutSparesKnownAddress(t *testing.T) {
	ts, db := newTestServer(t)
	db.CreateUser("ann", "correct-horse", "user")
	// The test client is loopback, a trusted proxy, so X-Forwarded-For sets
	// the client IP.
	login := func(ip, pw string) int {
		body, _ := json.Marshal(map[string]string{"username": "ann", "password": pw})
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", ip)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := login("10.0.0.1", "correct-horse"); code != 200 {
		t.Fatalf("first login: %d", code)
	}
	for i := range 20 {
		if code := login(fmt.Sprintf("10.1.0.%d", i+1), "wrong"); code != 401 {
			t.Fatalf("guess %d: %d", i+1, code)
		}
	}
	if code := login("10.9.9.9", "correct-horse"); code != 429 {
		t.Errorf("new address after the username lockout: %d, want 429", code)
	}
	if code := login("10.0.0.1", "correct-horse"); code != 200 {
		t.Errorf("known address after the username lockout: %d, want 200", code)
	}
}
