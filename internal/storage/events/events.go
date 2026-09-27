package events

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

// Repository holds the SQL of the events table.
type Repository struct {
	db DBTX
}

// New returns a repository over a connection or transaction.
func New(db DBTX) *Repository {
	return &Repository{db: db}
}

// Write appends events to the journal.
func (r *Repository) Write(ctx context.Context, events []model.Event) error {
	for _, e := range events {
		if _, err := r.db.ExecContext(ctx, `INSERT INTO events (at, device, kind, detail) VALUES (?, ?, ?, ?)`,
			e.At.Unix(), nullable(string(e.Device)), string(e.Kind), nullable(e.Detail)); err != nil {
			return fmt.Errorf("events: write %s: %w", e.Kind, err)
		}
	}
	return nil
}

// Since returns at most limit events at or after a moment, newest first.
func (r *Repository) Since(ctx context.Context, since time.Time, limit int) ([]model.Event, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT at, device, kind, detail FROM events
		WHERE at >= ? ORDER BY at DESC, id DESC LIMIT ?`, since.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("events: since: %w", err)
	}
	defer rows.Close()
	var out []model.Event
	for rows.Next() {
		var e model.Event
		var at int64
		var device, detail sql.NullString
		if err := rows.Scan(&at, &device, &e.Kind, &detail); err != nil {
			return nil, fmt.Errorf("events: scan: %w", err)
		}
		e.At, e.Device, e.Detail = time.Unix(at, 0), model.DeviceID(device.String), detail.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: since: %w", err)
	}
	return out, nil
}

// Prune deletes events older than a moment and reports how many.
func (r *Repository) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM events WHERE at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("events: prune: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("events: prune: %w", err)
	}
	return n, nil
}

func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
