package store

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open("file:" + filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEnvironments(t *testing.T) {
	s := openTest(t)

	// First start seeds "local" as the default, and doing it again is a no-op.
	local, err := s.EnsureDefaultEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if local.Name != "local" || local.Kind != KindLocal || !local.IsDefault || local.URL != "" {
		t.Fatalf("unexpected seed: %+v", local)
	}
	if again, err := s.EnsureDefaultEnvironment(); err != nil || again.ID != local.ID {
		t.Fatalf("seed not idempotent: %v %+v", err, again)
	}

	// Validation.
	bad := []Environment{
		{Name: "Bad Name", Kind: KindLocal},
		{Name: "test", Kind: KindLocal}, // reserved
		{Name: "x", Kind: "ftp"},
		{Name: "x", Kind: KindTLS, URL: "http://h:2376"},
		{Name: "x", Kind: KindSSH, URL: "tcp://h"},
	}
	for _, e := range bad {
		if _, err := s.CreateEnvironment(e); err == nil {
			t.Errorf("expected %+v to be rejected", e)
		}
	}

	edge, err := s.CreateEnvironment(Environment{
		Name: "edge", Kind: KindTLS, URL: "tcp://10.0.0.5:2376", Secret: "enc",
		Labels: map[string]string{"region": "nbo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if edge.IsDefault || edge.Label != "edge" || edge.Labels["region"] != "nbo" || !edge.HasSecret() {
		t.Fatalf("unexpected create: %+v", edge)
	}
	if _, err := s.CreateEnvironment(Environment{Name: "edge", Kind: KindLocal}); err == nil {
		t.Fatal("expected duplicate name to fail")
	}

	// Update keeps name; label and url change.
	edge.Label, edge.URL = "Edge box", "tcp://10.0.0.6:2376"
	if edge, err = s.UpdateEnvironment(edge); err != nil || edge.URL != "tcp://10.0.0.6:2376" {
		t.Fatalf("update: %v %+v", err, edge)
	}

	// The default can't be removed; moving the default frees it.
	if err := s.DeleteEnvironment("local"); err == nil {
		t.Fatal("expected deleting the default to fail")
	}
	if err := s.SetDefaultEnvironment("edge"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DefaultEnvironment(); d.Name != "edge" {
		t.Fatalf("default is %q", d.Name)
	}
	if err := s.DeleteEnvironment("local"); err != nil {
		t.Fatal(err)
	}

	// Status: last_seen_at only moves when up.
	if err := s.SetEnvironmentStatus(edge.ID, StatusDown); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.EnvironmentByName("edge"); e.Status != StatusDown || e.LastSeenAt != "" {
		t.Fatalf("after down: %+v", e)
	}
	if err := s.SetEnvironmentStatus(edge.ID, StatusUp); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.EnvironmentByName("edge"); e.Status != StatusUp || e.LastSeenAt == "" {
		t.Fatalf("after up: %+v", e)
	}

	list, err := s.ListEnvironments()
	if err != nil || len(list) != 1 || list[0].Name != "edge" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if _, err := s.EnvironmentByName("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAgentTokens(t *testing.T) {
	s := openTest(t)
	s.EnsureDefaultEnvironment()
	edge, err := s.CreateEnvironment(Environment{Name: "edge", Kind: KindAgent})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.NewAgentToken(edge.ID)
	if err != nil || len(tok) < 20 || tok[:3] != "la_" {
		t.Fatalf("token: %v %q", err, tok)
	}
	if e, err := s.EnvironmentByAgentToken(tok); err != nil || e.Name != "edge" {
		t.Fatalf("lookup: %v %+v", err, e)
	}
	// Rotating replaces the old token.
	tok2, _ := s.NewAgentToken(edge.ID)
	if _, err := s.EnvironmentByAgentToken(tok); err != ErrNotFound {
		t.Fatalf("old token should stop working: %v", err)
	}
	if _, err := s.EnvironmentByAgentToken(tok2); err != nil {
		t.Fatal(err)
	}
	// Deleting the environment removes its token.
	s.DeleteEnvironment("edge")
	if _, err := s.EnvironmentByAgentToken(tok2); err != ErrNotFound {
		t.Fatalf("token should go with the environment: %v", err)
	}
}
