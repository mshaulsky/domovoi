package devices

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

// Repository holds the SQL of the devices table.
type Repository struct {
	db DBTX
}

// New returns a repository over a connection or transaction.
func New(db DBTX) *Repository {
	return &Repository{db: db}
}

// Upsert records the devices as seen at `at`: new ones get both stamps,
// known ones keep first_seen and take the current name, room and kind.
func (r *Repository) Upsert(ctx context.Context, devices []model.Device, at time.Time) error {
	const q = `INSERT INTO devices (id, source, name, room, kind, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, room = excluded.room,
			kind = excluded.kind, last_seen = excluded.last_seen`
	for _, d := range devices {
		if _, err := r.db.ExecContext(ctx, q, string(d.ID), d.ID.Source(), d.Name, nullable(d.Room), string(d.Kind), at.Unix(), at.Unix()); err != nil {
			return fmt.Errorf("devices: upsert %s: %w", d.ID, err)
		}
	}
	return nil
}

// All lists every device ever seen, by ID.
func (r *Repository) All(ctx context.Context) ([]model.Device, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, room, kind FROM devices ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("devices: list: %w", err)
	}
	defer rows.Close()
	var out []model.Device
	for rows.Next() {
		var d model.Device
		var room sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &room, &d.Kind); err != nil {
			return nil, fmt.Errorf("devices: scan: %w", err)
		}
		d.Room = room.String
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("devices: list: %w", err)
	}
	return out, nil
}

// Prune deletes devices last seen before a moment that have no series left
// — retired devices whose history has aged out — and reports how many.
func (r *Repository) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM devices WHERE last_seen < ?
		AND NOT EXISTS (SELECT 1 FROM series WHERE series.device = devices.id)`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("devices: prune: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("devices: prune: %w", err)
	}
	return n, nil
}

// nullable stores an empty room as NULL, so "no room" is one value.
func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
