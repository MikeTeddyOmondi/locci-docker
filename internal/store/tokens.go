package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

// DefaultTokenTTL is how long a token lives after its last use unless
// LOCCI_TOKEN_TTL_DAYS says otherwise.
const DefaultTokenTTL = 30 * 24 * time.Hour

// Token is an API token as listed to its owner: never the token itself.
type Token struct {
	ID       string `json:"id"` // the first 16 hex characters of its hash
	Name     string `json:"name"`
	Created  string `json:"created_at"`
	LastUsed string `json:"last_used_at,omitempty"`
	Expires  string `json:"expires_at,omitempty"` // empty when tokens don't expire
	Current  bool   `json:"current"`              // the token making the request
}

var tokenID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// sqlTime formats t like SQLite's CURRENT_TIMESTAMP, so the two compare as text.
func sqlTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

// parseTime reads a timestamp as the driver returns it (RFC 3339) or as
// SQLite stores it.
func parseTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// cutoff is the oldest last use a live token can have ("" = no expiry).
func (s *Store) cutoff() string {
	if s.TokenTTL <= 0 {
		return ""
	}
	return sqlTime(time.Now().Add(-s.TokenTTL))
}

// NewToken creates an API token. Only its hash is stored. Expired tokens are
// cleared out at the same time.
func (s *Store) NewToken(userID int64, name string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := "lc_" + hex.EncodeToString(b)
	if _, err := s.DB.Exec(`INSERT INTO tokens (token_hash, user_id, name) VALUES (?, ?, ?)`,
		hashToken(token), userID, name); err != nil {
		return "", err
	}
	if c := s.cutoff(); c != "" {
		_, _ = s.DB.Exec(`DELETE FROM tokens WHERE COALESCE(last_used_at, created_at) < ?`, c)
	}
	return token, nil
}

// UserByToken returns the token's user if the token exists and hasn't
// expired, and slides its expiry forward (at most one write an hour).
func (s *Store) UserByToken(token string) (User, error) {
	h, c := hashToken(token), s.cutoff()
	u, err := s.scanUser(s.DB.QueryRow(`SELECT `+userColumns+`
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ? AND (? = '' OR COALESCE(t.last_used_at, t.created_at) >= ?)`, h, c, c))
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	_, _ = s.DB.Exec(`UPDATE tokens SET last_used_at = ?
		WHERE token_hash = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		sqlTime(now), h, sqlTime(now.Add(-time.Hour)))
	return u, nil
}

func (s *Store) DeleteToken(token string) error {
	_, err := s.DB.Exec(`DELETE FROM tokens WHERE token_hash = ?`, hashToken(token))
	return err
}

// Tokens lists a user's live tokens, newest first; current marks the caller's.
func (s *Store) Tokens(userID int64, current string) ([]Token, error) {
	c := s.cutoff()
	rows, err := s.DB.Query(`SELECT token_hash, name, created_at, COALESCE(last_used_at, '')
		FROM tokens WHERE user_id = ? AND (? = '' OR COALESCE(last_used_at, created_at) >= ?)
		ORDER BY created_at DESC`, userID, c, c)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cur := hashToken(current)
	out := []Token{}
	for rows.Next() {
		var h string
		var t Token
		if err := rows.Scan(&h, &t.Name, &t.Created, &t.LastUsed); err != nil {
			return nil, err
		}
		t.ID, t.Current = h[:16], h == cur
		if s.TokenTTL > 0 {
			from := t.LastUsed
			if from == "" {
				from = t.Created
			}
			if at, ok := parseTime(from); ok {
				t.Expires = at.Add(s.TokenTTL).UTC().Format(time.RFC3339)
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken deletes one of the user's tokens by its listed ID.
func (s *Store) RevokeToken(userID int64, id string) error {
	if !tokenID.MatchString(id) {
		return fmt.Errorf("%w: token id %q", ErrNotFound, id)
	}
	res, err := s.DB.Exec(`DELETE FROM tokens WHERE user_id = ? AND substr(token_hash, 1, 16) = ?`, userID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
