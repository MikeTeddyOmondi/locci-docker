package store

import (
	"fmt"
	"slices"
)

// Per-environment access. An environment is open to every signed-in user
// until an admin restricts it; then only the users on its list (and admins)
// may use it. Restriction is its own flag, so removing the last listed user
// leaves the environment admin-only rather than opening it to everyone.

// Access is who may use an environment.
type Access struct {
	Restricted bool     `json:"restricted"`
	Users      []string `json:"users"` // sorted; only meaningful when Restricted
}

// EnvironmentAccess returns an environment's access settings.
func (s *Store) EnvironmentAccess(envID int64) (Access, error) {
	a := Access{Users: []string{}}
	if err := s.DB.QueryRow(`SELECT access_restricted FROM environments WHERE id = ?`, envID).Scan(&a.Restricted); err != nil {
		return a, err
	}
	rows, err := s.DB.Query(`SELECT u.username FROM environment_users eu
		JOIN users u ON u.id = eu.user_id WHERE eu.environment_id = ? ORDER BY u.username`, envID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return a, err
		}
		a.Users = append(a.Users, name)
	}
	return a, rows.Err()
}

// SetEnvironmentAccess opens an environment to everyone (restricted false;
// the list is cleared) or limits it to usernames (possibly none: admins
// only). Unknown usernames are refused.
func (s *Store) SetEnvironmentAccess(envID int64, restricted bool, usernames []string) error {
	var ids []int64
	if restricted {
		for _, name := range slices.Compact(slices.Sorted(slices.Values(usernames))) {
			if name == "" {
				continue
			}
			u, err := s.UserByName(name)
			if err != nil {
				return fmt.Errorf("user %q: %w", name, err)
			}
			ids = append(ids, u.ID)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE environments SET access_restricted = ? WHERE id = ?`, restricted, envID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM environment_users WHERE environment_id = ?`, envID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO environment_users (environment_id, user_id) VALUES (?, ?)`, envID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CanUseEnvironment reports whether a (non-admin) user may use an environment.
func (s *Store) CanUseEnvironment(envID, userID int64) (bool, error) {
	var restricted bool
	var listed int
	err := s.DB.QueryRow(`SELECT e.access_restricted,
		(SELECT COUNT(*) FROM environment_users eu WHERE eu.environment_id = e.id AND eu.user_id = ?)
		FROM environments e WHERE e.id = ?`, userID, envID).Scan(&restricted, &listed)
	if err != nil {
		return false, err
	}
	return !restricted || listed > 0, nil
}
