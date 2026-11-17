// Package store keeps users, API tokens, Docker environments and the audit log in libsql.
//
// DATABASE_URL can be a local file ("file:locci.db") or a remote
// libsql/Turso URL ("libsql://<db>.turso.io?authToken=...").
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	_ "github.com/tursodatabase/go-libsql"
	"golang.org/x/crypto/bcrypt"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"` // "admin" or "user"
	Created  string `json:"created_at"`
}

func (u User) IsAdmin() bool { return u.Role == "admin" }

type AuditEntry struct {
	ID       int64  `json:"id"`
	At       string `json:"at"`
	Username string `json:"username"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Status   int    `json:"status"`
}

type Store struct {
	DB *sql.DB
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY,
		username      TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role          TEXT NOT NULL DEFAULT 'user',
		created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS tokens (
		token_hash TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name       TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS audit (
		id       INTEGER PRIMARY KEY,
		at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		username TEXT NOT NULL,
		method   TEXT NOT NULL,
		path     TEXT NOT NULL,
		status   INTEGER NOT NULL
	)`,
	// Docker hosts. secret is the TLS client key or SSH private key, encrypted
	// with LOCCI_SECRET_KEY before it gets here (see internal/secret).
	`CREATE TABLE IF NOT EXISTS environments (
		id           INTEGER PRIMARY KEY,
		name         TEXT NOT NULL UNIQUE,
		label        TEXT NOT NULL DEFAULT '',
		kind         TEXT NOT NULL,
		url          TEXT NOT NULL DEFAULT '',
		tls_ca       TEXT NOT NULL DEFAULT '',
		tls_cert     TEXT NOT NULL DEFAULT '',
		secret       TEXT NOT NULL DEFAULT '',
		ssh_host_key TEXT NOT NULL DEFAULT '',
		labels       TEXT NOT NULL DEFAULT '{}',
		is_default   INTEGER NOT NULL DEFAULT 0,
		status       TEXT NOT NULL DEFAULT 'unknown',
		last_seen_at TEXT,
		created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	// At most one default environment.
	`CREATE UNIQUE INDEX IF NOT EXISTS environments_one_default
		ON environments (is_default) WHERE is_default = 1`,
}

// Open connects and creates the tables if needed.
func Open(url string) (*Store, error) {
	db, err := sql.Open("libsql", url)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", url, err)
	}
	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// ---- users ----

func (s *Store) CreateUser(username, password, role string) (User, error) {
	if username == "" || len(password) < 8 {
		return User{}, errors.New("username required and password must be at least 8 characters")
	}
	if role != "admin" {
		role = "user"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	if _, err := s.DB.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, string(hash), role); err != nil {
		return User{}, err
	}
	return s.UserByName(username)
}

func (s *Store) UserByName(username string) (User, error) {
	return s.scanUser(s.DB.QueryRow(
		`SELECT id, username, role, created_at FROM users WHERE username = ?`, username))
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.DB.Query(`SELECT id, username, role, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Created); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) DeleteUser(username string) error {
	u, err := s.UserByName(username)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM tokens WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
	return err
}

func (s *Store) SetPassword(username, password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET password_hash = ? WHERE username = ?`, string(hash), username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckPassword returns the user when the password matches.
func (s *Store) CheckPassword(username, password string) (User, error) {
	var hash string
	err := s.DB.QueryRow(`SELECT password_hash FROM users WHERE username = ?`, username).Scan(&hash)
	if err != nil {
		return User{}, ErrNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrNotFound
	}
	return s.UserByName(username)
}

func (s *Store) scanUser(row *sql.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Role, &u.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// ---- tokens ----

// NewToken creates an API token. Only its hash is stored.
func (s *Store) NewToken(userID int64, name string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := "lc_" + hex.EncodeToString(b)
	_, err := s.DB.Exec(`INSERT INTO tokens (token_hash, user_id, name) VALUES (?, ?, ?)`,
		hashToken(token), userID, name)
	return token, err
}

func (s *Store) UserByToken(token string) (User, error) {
	return s.scanUser(s.DB.QueryRow(`
		SELECT u.id, u.username, u.role, u.created_at
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ?`, hashToken(token)))
}

func (s *Store) DeleteToken(token string) error {
	_, err := s.DB.Exec(`DELETE FROM tokens WHERE token_hash = ?`, hashToken(token))
	return err
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// ---- audit ----

func (s *Store) Audit(username, method, path string, status int) error {
	_, err := s.DB.Exec(`INSERT INTO audit (username, method, path, status) VALUES (?, ?, ?, ?)`,
		username, method, path, status)
	return err
}

func (s *Store) ListAudit(limit int) ([]AuditEntry, error) {
	rows, err := s.DB.Query(
		`SELECT id, at, username, method, path, status FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEntry{}
	for rows.Next() {
		var a AuditEntry
		if err := rows.Scan(&a.ID, &a.At, &a.Username, &a.Method, &a.Path, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
