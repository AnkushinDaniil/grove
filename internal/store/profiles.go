package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const upsertProfileSQL = `
INSERT INTO profiles (id, driver, name, config_dir, is_default, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	driver = excluded.driver,
	name = excluded.name,
	config_dir = excluded.config_dir,
	is_default = excluded.is_default
`

// SaveProfile upserts one profile.
func (s *Store) SaveProfile(ctx context.Context, p core.Profile) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, upsertProfileSQL,
			string(p.ID), p.Driver, p.Name, p.ConfigDir, p.IsDefault, msFromTime(p.CreatedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert profile %s: %w", p.ID, err)
		}
		return nil
	})
}

const selectProfilesSQL = `
SELECT id, driver, name, config_dir, is_default, created_at
FROM profiles
ORDER BY created_at ASC
`

// ListProfiles returns every registered profile, oldest first.
func (s *Store) ListProfiles(ctx context.Context) ([]core.Profile, error) {
	rows, err := s.db.QueryContext(ctx, selectProfilesSQL)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	profiles, err := collect(rows, scanProfile)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	return profiles, nil
}

// DeleteProfile removes a profile by ID. Deleting an ID that does not exist
// is not an error.
func (s *Store) DeleteProfile(ctx context.Context, id core.ProfileID) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM profiles WHERE id = ?", string(id)); err != nil {
			return fmt.Errorf("delete profile %s: %w", id, err)
		}
		return nil
	})
}
