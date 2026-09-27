package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Repository holds the SQL of the settings table.
type Repository struct {
	db DBTX
}

// New returns a repository over a connection or transaction.
func New(db DBTX) *Repository {
	return &Repository{db: db}
}

// Get returns a setting's value; ok is false when the key is not set.
func (r *Repository) Get(ctx context.Context, key string) (value string, ok bool, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("settings: get %s: %w", key, err)
	}
	return value, true, nil
}

// Set writes a setting, replacing an existing value.
func (r *Repository) Set(ctx context.Context, key, value string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, at.Unix()); err != nil {
		return fmt.Errorf("settings: set %s: %w", key, err)
	}
	return nil
}

// Delete removes a setting, so the file's default applies again.
func (r *Repository) Delete(ctx context.Context, key string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("settings: delete %s: %w", key, err)
	}
	return nil
}

// All returns every setting.
func (r *Repository) All(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("settings: scan: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings: list: %w", err)
	}
	return out, nil
}
