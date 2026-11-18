package store

import (
	"errors"
	"testing"
)

func TestProviders(t *testing.T) {
	s := openTest(t)

	for _, bad := range []Provider{
		{Slug: "Bad Slug", IssuerURL: "https://id.example.com", ClientID: "c"},
		{Slug: "kc", IssuerURL: "http://id.example.com", ClientID: "c"}, // plain http off localhost
		{Slug: "kc", IssuerURL: "not a url", ClientID: "c"},
		{Slug: "kc", IssuerURL: "https://id.example.com"}, // no client id
	} {
		if _, err := s.CreateProvider(bad); err == nil {
			t.Errorf("expected %+v to be rejected", bad)
		}
	}

	p, err := s.CreateProvider(Provider{Slug: "kc", IssuerURL: "https://id.example.com/realms/main", ClientID: "locci",
		ClientSecret: "v1:sealed", AllowedDomains: " Example.com, @corp.io ,"})
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "kc" || p.DefaultRole != "user" || p.GroupsClaim != "groups" || p.Scopes != "openid email profile" || p.Enabled {
		t.Fatalf("defaults: %+v", p)
	}
	if d := p.Domains(); len(d) != 2 || d[0] != "example.com" || d[1] != "corp.io" {
		t.Fatalf("domains: %v", d)
	}
	if _, err := s.CreateProvider(Provider{Slug: "dev", IssuerURL: "http://127.0.0.1:5556/dex", ClientID: "c"}); err != nil {
		t.Fatalf("http on localhost should be allowed: %v", err)
	}

	p.Enabled, p.DisplayName = true, "Keycloak"
	if p, err = s.UpdateProvider(p); err != nil || !p.Enabled || p.DisplayName != "Keycloak" {
		t.Fatalf("update: %v %+v", err, p)
	}
	if on, _ := s.ListProviders(true); len(on) != 1 || on[0].Slug != "kc" {
		t.Fatalf("enabled list: %+v", on)
	}
	if all, _ := s.ListProviders(false); len(all) != 2 {
		t.Fatalf("all: %d", len(all))
	}

	// SSO-only users exist without a password and can't log in with one.
	u, err := s.CreateSSOUser("ann@example.com", "user")
	if err != nil || u.HasPassword {
		t.Fatalf("sso user: %v %+v", err, u)
	}
	if _, err := s.CheckPassword("ann@example.com", ""); err == nil {
		t.Fatal("an empty password must never match an SSO-only user")
	}
	if err := s.LinkIdentity(p.ID, "sub-1", u.ID, "ann@example.com"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.UserByIdentity(p.ID, "sub-1"); err != nil || got.ID != u.ID {
		t.Fatalf("identity lookup: %v %+v", err, got)
	}
	if err := s.LinkIdentity(p.ID, "sub-1", u.ID, ""); err == nil {
		t.Fatal("expected a duplicate subject to fail")
	}
	if err := s.SetRole(u.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UserByName("ann@example.com"); !got.IsAdmin() {
		t.Fatal("expected admin after SetRole")
	}

	// Giving the user a password later works as usual.
	if err := s.SetPassword("ann@example.com", "a-real-password"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CheckPassword("ann@example.com", "a-real-password"); err != nil || !got.HasPassword {
		t.Fatalf("password after SetPassword: %v %+v", err, got)
	}

	// Deleting the provider removes its identities but keeps the user.
	if err := s.DeleteProvider("kc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByIdentity(p.ID, "sub-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("identity should be gone: %v", err)
	}
	if _, err := s.UserByName("ann@example.com"); err != nil {
		t.Fatal("user should remain")
	}
}

func TestPasswordLoginSetting(t *testing.T) {
	s := openTest(t)
	if !s.PasswordLogin() {
		t.Fatal("password login must be on by default")
	}
	if err := s.SetPasswordLogin(false); err != nil || s.PasswordLogin() {
		t.Fatal("expected off")
	}
	if err := s.SetPasswordLogin(true); err != nil || !s.PasswordLogin() {
		t.Fatal("expected on again")
	}
}

func TestDeleteUserRemovesIdentities(t *testing.T) {
	s := openTest(t)
	p, _ := s.CreateProvider(Provider{Slug: "kc", IssuerURL: "https://id.example.com", ClientID: "c"})
	u, _ := s.CreateSSOUser("bob", "user")
	s.LinkIdentity(p.ID, "sub-2", u.ID, "")
	if err := s.DeleteUser("bob"); err != nil {
		t.Fatal(err)
	}
	// A new account may take over the subject after the old one is gone.
	u2, _ := s.CreateSSOUser("bob2", "user")
	if err := s.LinkIdentity(p.ID, "sub-2", u2.ID, ""); err != nil {
		t.Fatalf("subject should be free again: %v", err)
	}
}
