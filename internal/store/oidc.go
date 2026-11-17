package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Provider is an OpenID Connect identity provider. ClientSecret is already
// encrypted by the caller and never leaves the server.
type Provider struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"` // used in URLs: /auth/oidc/<slug>/start
	DisplayName     string `json:"display_name"`
	IssuerURL       string `json:"issuer_url"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"-"`
	Scopes          string `json:"scopes"` // space separated; openid is always added
	Enabled         bool   `json:"enabled"`
	AutoCreateUsers bool   `json:"auto_create_users"`
	DefaultRole     string `json:"default_role"`          // role for new users: user or admin
	AdminGroup      string `json:"admin_group"`           // members get admin; empty = roles not synced
	GroupsClaim     string `json:"groups_claim"`          // claim holding group names
	AllowedDomains  string `json:"allowed_email_domains"` // comma separated; empty = any
	CreatedAt       string `json:"created_at"`
}

// HasSecret reports whether a client secret is stored (public clients have none).
func (p Provider) HasSecret() bool { return p.ClientSecret != "" }

// Domains returns the allowed email domains, lowercased.
func (p Provider) Domains() []string {
	var out []string
	for _, d := range strings.Split(p.AllowedDomains, ",") {
		if d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "@"))); d != "" {
			out = append(out, d)
		}
	}
	return out
}

const providerColumns = `id, slug, display_name, issuer_url, client_id, client_secret, scopes,
	enabled, auto_create_users, default_role, admin_group, groups_claim, allowed_domains, created_at`

func scanProvider(row interface{ Scan(...any) error }) (Provider, error) {
	var p Provider
	err := row.Scan(&p.ID, &p.Slug, &p.DisplayName, &p.IssuerURL, &p.ClientID, &p.ClientSecret, &p.Scopes,
		&p.Enabled, &p.AutoCreateUsers, &p.DefaultRole, &p.AdminGroup, &p.GroupsClaim, &p.AllowedDomains, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, ErrNotFound
	}
	return p, err
}

func validateProvider(p *Provider) error {
	if !nameRE.MatchString(p.Slug) {
		return errors.New("slug must be lowercase letters, digits and dashes (max 63)")
	}
	u, err := url.Parse(p.IssuerURL)
	if err != nil || u.Host == "" {
		return errors.New("issuer URL must be absolute, e.g. https://keycloak.example.com/realms/main")
	}
	// Plain HTTP only for issuers on this machine (development and tests).
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("issuer URL must use https (http is allowed only for localhost)")
	}
	if p.ClientID == "" {
		return errors.New("client ID is required")
	}
	if p.DisplayName == "" {
		p.DisplayName = p.Slug
	}
	if p.DefaultRole != "admin" {
		p.DefaultRole = "user"
	}
	if p.GroupsClaim == "" {
		p.GroupsClaim = "groups"
	}
	if strings.TrimSpace(p.Scopes) == "" {
		p.Scopes = "openid email profile"
	}
	return nil
}

func (s *Store) CreateProvider(p Provider) (Provider, error) {
	if err := validateProvider(&p); err != nil {
		return Provider{}, err
	}
	_, err := s.DB.Exec(`INSERT INTO oidc_providers (slug, display_name, issuer_url, client_id, client_secret,
		scopes, enabled, auto_create_users, default_role, admin_group, groups_claim, allowed_domains)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Slug, p.DisplayName, p.IssuerURL, p.ClientID, p.ClientSecret, p.Scopes, p.Enabled,
		p.AutoCreateUsers, p.DefaultRole, p.AdminGroup, p.GroupsClaim, p.AllowedDomains)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Provider{}, fmt.Errorf("provider %q already exists", p.Slug)
		}
		return Provider{}, err
	}
	return s.ProviderBySlug(p.Slug)
}

// UpdateProvider saves everything except the slug.
func (s *Store) UpdateProvider(p Provider) (Provider, error) {
	if err := validateProvider(&p); err != nil {
		return Provider{}, err
	}
	res, err := s.DB.Exec(`UPDATE oidc_providers SET display_name = ?, issuer_url = ?, client_id = ?,
		client_secret = ?, scopes = ?, enabled = ?, auto_create_users = ?, default_role = ?,
		admin_group = ?, groups_claim = ?, allowed_domains = ? WHERE slug = ?`,
		p.DisplayName, p.IssuerURL, p.ClientID, p.ClientSecret, p.Scopes, p.Enabled, p.AutoCreateUsers,
		p.DefaultRole, p.AdminGroup, p.GroupsClaim, p.AllowedDomains, p.Slug)
	if err != nil {
		return Provider{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Provider{}, ErrNotFound
	}
	return s.ProviderBySlug(p.Slug)
}

func (s *Store) ProviderBySlug(slug string) (Provider, error) {
	return scanProvider(s.DB.QueryRow(`SELECT `+providerColumns+` FROM oidc_providers WHERE slug = ?`, slug))
}

// ListProviders returns all providers, or only enabled ones.
func (s *Store) ListProviders(enabledOnly bool) ([]Provider, error) {
	q := `SELECT ` + providerColumns + ` FROM oidc_providers`
	if enabledOnly {
		q += ` WHERE enabled = 1`
	}
	rows, err := s.DB.Query(q + ` ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProvider removes a provider and the identities linked through it.
// The users stay; SSO-only users can then only sign in once given a password.
func (s *Store) DeleteProvider(slug string) error {
	p, err := s.ProviderBySlug(slug)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM user_identities WHERE provider_id = ?`, p.ID); err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM oidc_providers WHERE id = ?`, p.ID)
	return err
}

// ---- identities ----

// UserByIdentity finds the user linked to a provider subject.
func (s *Store) UserByIdentity(providerID int64, subject string) (User, error) {
	return s.scanUser(s.DB.QueryRow(`SELECT `+userColumns+` FROM user_identities i
		JOIN users u ON u.id = i.user_id WHERE i.provider_id = ? AND i.subject = ?`, providerID, subject))
}

// LinkIdentity connects a provider subject to a user.
func (s *Store) LinkIdentity(providerID int64, subject string, userID int64, email string) error {
	_, err := s.DB.Exec(`INSERT INTO user_identities (provider_id, subject, user_id, email) VALUES (?, ?, ?, ?)`,
		providerID, subject, userID, email)
	return err
}

// ---- settings ----

func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// PasswordLogin reports whether username/password login is allowed (the default).
func (s *Store) PasswordLogin() bool { return s.Setting("password_login", "on") != "off" }

func (s *Store) SetPasswordLogin(on bool) error {
	v := "off"
	if on {
		v = "on"
	}
	return s.SetSetting("password_login", v)
}
