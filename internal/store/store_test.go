package store

import (
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	s, err := Open("file:" + filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateUser("mike", "short", "admin"); err == nil {
		t.Fatal("expected short password to be rejected")
	}

	u, err := s.CreateUser("mike", "correct-horse", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin() {
		t.Fatal("expected admin")
	}

	if _, err := s.CheckPassword("mike", "wrong-password"); err == nil {
		t.Fatal("expected wrong password to fail")
	}
	if _, err := s.CheckPassword("mike", "correct-horse"); err != nil {
		t.Fatal(err)
	}

	tok, err := s.NewToken(u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByToken(tok)
	if err != nil || got.Username != "mike" {
		t.Fatalf("token lookup: %v %+v", err, got)
	}
	if err := s.DeleteToken(tok); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByToken(tok); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	if err := s.Audit("mike", "POST", "/api/v1/containers", 201); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListAudit(10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit: %v %d", err, len(entries))
	}

	if err := s.DeleteUser("mike"); err != nil {
		t.Fatal(err)
	}
	if users, _ := s.ListUsers(); len(users) != 0 {
		t.Fatalf("expected no users, got %d", len(users))
	}
}
