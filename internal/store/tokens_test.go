package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenExpiry(t *testing.T) {
	db, err := Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, _ := db.CreateUser("ann", "correct-horse", "user")
	db.TokenTTL = time.Hour

	tok, _ := db.NewToken(u.ID, "laptop")
	if _, err := db.UserByToken(tok); err != nil {
		t.Fatalf("fresh token: %v", err)
	}
	// Last used two hours ago: expired.
	db.DB.Exec(`UPDATE tokens SET last_used_at = ?`, sqlTime(time.Now().Add(-2*time.Hour)))
	if _, err := db.UserByToken(tok); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired token: %v", err)
	}
	if list, _ := db.Tokens(u.ID, tok); len(list) != 0 {
		t.Fatalf("expired token listed: %v", list)
	}
	// No expiry: the same token works again.
	db.TokenTTL = 0
	if _, err := db.UserByToken(tok); err != nil {
		t.Fatalf("TTL 0: %v", err)
	}
	// A new login clears expired tokens out.
	db.TokenTTL = time.Hour
	db.DB.Exec(`UPDATE tokens SET last_used_at = ?`, sqlTime(time.Now().Add(-2*time.Hour)))
	db.NewToken(u.ID, "phone")
	var n int
	db.DB.QueryRow(`SELECT COUNT(*) FROM tokens`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d tokens after pruning, want 1", n)
	}
}

func TestListAndRevokeTokens(t *testing.T) {
	db, err := Open("file:" + filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ann, _ := db.CreateUser("ann", "correct-horse", "user")
	bob, _ := db.CreateUser("bob", "correct-horse", "user")
	a1, _ := db.NewToken(ann.ID, "laptop")
	a2, _ := db.NewToken(ann.ID, "phone")
	db.NewToken(bob.ID, "bob")

	list, err := db.Tokens(ann.ID, a1)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}
	var cur, other Token
	for _, tk := range list {
		if tk.Current {
			cur = tk
		} else {
			other = tk
		}
	}
	if cur.Name != "laptop" || cur.Expires == "" || len(cur.ID) != 16 {
		t.Fatalf("current: %+v", cur)
	}
	// Bob can't revoke Ann's token; Ann can.
	if err := db.RevokeToken(bob.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke someone else's: %v", err)
	}
	if err := db.RevokeToken(ann.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UserByToken(a2); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked token still works")
	}
	if err := db.RevokeToken(ann.ID, "x' OR 1=1 --"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}
