package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Connection kinds for an environment.
const (
	KindLocal  = "local"   // DOCKER_HOST or the local socket
	KindTLS    = "tcp+tls" // Docker's TLS port with client certificates
	KindSSH    = "ssh"     // Docker socket reached over SSH
	KindAgent  = "agent"   // a `locci agent` on the host dials out to the server
	StatusUp   = "up"
	StatusDown = "down"
)

// Environment is one Docker host. Secret holds the TLS client key or the SSH
// private key, already encrypted by the caller; it never leaves the server.
type Environment struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`  // URL/CLI slug, fixed once created
	Label      string            `json:"label"` // display name
	Kind       string            `json:"kind"`
	URL        string            `json:"url"`
	TLSCA      string            `json:"tls_ca,omitempty"`   // PEM, not secret
	TLSCert    string            `json:"tls_cert,omitempty"` // PEM, not secret
	Secret     string            `json:"-"`
	SSHHostKey string            `json:"ssh_host_key,omitempty"` // pinned on first successful connect
	Labels     map[string]string `json:"labels"`
	IsDefault  bool              `json:"is_default"`
	Status     string            `json:"status"` // unknown, up, down
	LastSeenAt string            `json:"last_seen_at"`
	CreatedAt  string            `json:"created_at"`
}

// HasSecret reports whether a key is stored, without exposing it.
func (e Environment) HasSecret() bool { return e.Secret != "" }

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// reserved names would clash with fixed API paths (POST /environments/test).
var reserved = map[string]bool{"test": true}

// ValidEnvironmentName reports whether name is a usable slug.
func ValidEnvironmentName(name string) bool { return nameRE.MatchString(name) && !reserved[name] }

const envColumns = `id, name, label, kind, url, tls_ca, tls_cert, secret, ssh_host_key,
	labels, is_default, status, COALESCE(last_seen_at, ''), created_at`

// envColumnsQualified is envColumns for a query that aliases environments as e.
const envColumnsQualified = `e.id, e.name, e.label, e.kind, e.url, e.tls_ca, e.tls_cert, e.secret,
	e.ssh_host_key, e.labels, e.is_default, e.status, COALESCE(e.last_seen_at, ''), e.created_at`

func scanEnvironment(row interface{ Scan(...any) error }) (Environment, error) {
	var e Environment
	var labels string
	err := row.Scan(&e.ID, &e.Name, &e.Label, &e.Kind, &e.URL, &e.TLSCA, &e.TLSCert, &e.Secret,
		&e.SSHHostKey, &labels, &e.IsDefault, &e.Status, stamp{&e.LastSeenAt}, stamp{&e.CreatedAt})
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	if err != nil {
		return Environment{}, err
	}
	e.Labels = map[string]string{}
	_ = json.Unmarshal([]byte(labels), &e.Labels)
	return e, nil
}

func validateEnvironment(e Environment) error {
	if !ValidEnvironmentName(e.Name) {
		return errors.New("name must be lowercase letters, digits and dashes (max 63), and not \"test\"")
	}
	switch e.Kind {
	case KindLocal:
	case KindTLS:
		if !strings.HasPrefix(e.URL, "tcp://") {
			return errors.New("tcp+tls url must look like tcp://host:2376")
		}
	case KindSSH:
		if !strings.HasPrefix(e.URL, "ssh://") {
			return errors.New("ssh url must look like ssh://user@host[:port]")
		}
	case KindAgent:
		// The agent connects to us; there is nothing to dial.
	default:
		return fmt.Errorf("unknown kind %q (use local, tcp+tls, ssh or agent)", e.Kind)
	}
	return nil
}

func labelsJSON(l map[string]string) string {
	if l == nil {
		l = map[string]string{}
	}
	b, _ := json.Marshal(l)
	return string(b)
}

// CreateEnvironment inserts e. The first environment ever created becomes the default.
func (s *Store) CreateEnvironment(e Environment) (Environment, error) {
	if err := validateEnvironment(e); err != nil {
		return Environment{}, err
	}
	if e.Label == "" {
		e.Label = e.Name
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM environments`).Scan(&n); err != nil {
		return Environment{}, err
	}
	_, err := s.DB.Exec(`INSERT INTO environments
		(name, label, kind, url, tls_ca, tls_cert, secret, ssh_host_key, labels, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Name, e.Label, e.Kind, e.URL, e.TLSCA, e.TLSCert, e.Secret, e.SSHHostKey,
		labelsJSON(e.Labels), n == 0)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Environment{}, fmt.Errorf("environment %q already exists", e.Name)
		}
		return Environment{}, err
	}
	return s.EnvironmentByName(e.Name)
}

// UpdateEnvironment saves everything except name, default flag and status.
func (s *Store) UpdateEnvironment(e Environment) (Environment, error) {
	if err := validateEnvironment(e); err != nil {
		return Environment{}, err
	}
	res, err := s.DB.Exec(`UPDATE environments SET label = ?, kind = ?, url = ?, tls_ca = ?,
		tls_cert = ?, secret = ?, ssh_host_key = ?, labels = ? WHERE name = ?`,
		e.Label, e.Kind, e.URL, e.TLSCA, e.TLSCert, e.Secret, e.SSHHostKey, labelsJSON(e.Labels), e.Name)
	if err != nil {
		return Environment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Environment{}, ErrNotFound
	}
	return s.EnvironmentByName(e.Name)
}

func (s *Store) EnvironmentByName(name string) (Environment, error) {
	return scanEnvironment(s.DB.QueryRow(`SELECT `+envColumns+` FROM environments WHERE name = ?`, name))
}

func (s *Store) DefaultEnvironment() (Environment, error) {
	return scanEnvironment(s.DB.QueryRow(`SELECT ` + envColumns + ` FROM environments WHERE is_default = 1`))
}

// ListEnvironments returns the default first, then by name.
func (s *Store) ListEnvironments() ([]Environment, error) {
	rows, err := s.DB.Query(`SELECT ` + envColumns + ` FROM environments ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Environment{}
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEnvironment removes a non-default environment.
func (s *Store) DeleteEnvironment(name string) error {
	e, err := s.EnvironmentByName(name)
	if err != nil {
		return err
	}
	if e.IsDefault {
		return errors.New("the default environment can't be removed; make another one the default first")
	}
	if _, err := s.DB.Exec(`DELETE FROM agent_tokens WHERE environment_id = ?`, e.ID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM environment_users WHERE environment_id = ?`, e.ID); err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM environments WHERE id = ?`, e.ID)
	return err
}

// NewAgentToken creates (or replaces) the token an agent environment's
// `locci agent` connects with. Only its hash is stored; replacing it locks
// out an agent using the old token.
func (s *Store) NewAgentToken(envID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := "la_" + hex.EncodeToString(b)
	_, err := s.DB.Exec(`INSERT INTO agent_tokens (environment_id, token_hash) VALUES (?, ?)
		ON CONFLICT(environment_id) DO UPDATE SET token_hash = excluded.token_hash, created_at = CURRENT_TIMESTAMP`,
		envID, hashToken(token))
	return token, err
}

// EnvironmentByAgentToken finds the agent environment a token belongs to.
func (s *Store) EnvironmentByAgentToken(token string) (Environment, error) {
	return scanEnvironment(s.DB.QueryRow(`SELECT `+envColumnsQualified+` FROM agent_tokens a
		JOIN environments e ON e.id = a.environment_id WHERE a.token_hash = ? AND e.kind = ?`,
		hashToken(token), KindAgent))
}

// SetDefaultEnvironment moves the default flag to name in one transaction.
func (s *Store) SetDefaultEnvironment(name string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE environments SET is_default = 0 WHERE is_default = 1`); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE environments SET is_default = 1 WHERE name = ?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetEnvironmentStatus records a health check; last_seen_at moves only when up.
func (s *Store) SetEnvironmentStatus(id int64, status string) error {
	q := `UPDATE environments SET status = ? WHERE id = ?`
	if status == StatusUp {
		q = `UPDATE environments SET status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	_, err := s.DB.Exec(q, status, id)
	return err
}

// EnsureDefaultEnvironment creates the "local" environment on first start.
// Its empty URL means "DOCKER_HOST or the local socket", so existing installs
// behave exactly as before.
func (s *Store) EnsureDefaultEnvironment() (Environment, error) {
	if e, err := s.DefaultEnvironment(); err == nil {
		return e, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Environment{}, err
	}
	envs, err := s.ListEnvironments()
	if err != nil {
		return Environment{}, err
	}
	if len(envs) > 0 { // rows exist but none is default: promote the first
		return envs[0], s.SetDefaultEnvironment(envs[0].Name)
	}
	return s.CreateEnvironment(Environment{Name: "local", Label: "Local", Kind: KindLocal})
}
