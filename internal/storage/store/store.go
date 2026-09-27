package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/storage/devices"
	"github.com/mshaulsky/domovoi/internal/storage/events"
	"github.com/mshaulsky/domovoi/internal/storage/readings"
)

// Store composes the repositories over one connection pool.
type Store struct {
	db *sql.DB
}

// New returns a store over an opened database.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Persist writes one batch atomically: devices upserted as seen at `at`,
// readings written, events journalled. Nothing is written if any part
// fails.
func (s *Store) Persist(ctx context.Context, devs []model.Device, rs []model.Reading, evs []model.Event, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit; the error of a failed rollback is the original one
	if err := devices.New(tx).Upsert(ctx, devs, at); err != nil {
		return err
	}
	if err := readings.New(tx).Write(ctx, rs); err != nil {
		return err
	}
	if err := events.New(tx).Write(ctx, evs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// Restore returns every known device and the newest point of every
// series: the picture to seed the state with after a restart.
func (s *Store) Restore(ctx context.Context) ([]model.Device, []model.Reading, error) {
	devs, err := devices.New(s.db).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	rs, err := readings.New(s.db).LastAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	return devs, rs, nil
}

// Prune deletes readings and events older than a moment, then the devices
// unseen since then whose history is all gone, in one transaction, and
// reports how many rows went.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit; the error of a failed rollback is the original one
	var total int64
	for _, prune := range []func(context.Context, time.Time) (int64, error){
		readings.New(tx).Prune, events.New(tx).Prune, devices.New(tx).Prune,
	} {
		n, err := prune(ctx, before)
		if err != nil {
			return 0, err
		}
		total += n
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit: %w", err)
	}
	return total, nil
}

// Extremes returns the lowest and highest value of a numeric series since
// a moment; ok is false without data.
func (s *Store) Extremes(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (lo, hi float64, ok bool, err error) {
	l, h, ok, err := readings.New(s.db).Extremes(ctx, id, m, since)
	if err != nil || !ok {
		return 0, 0, false, err
	}
	return l.Value.Num, h.Value.Num, true, nil
}

// Trend returns how much a numeric series changed since a moment: the
// newest value minus the value back then; ok is false without both.
func (s *Store) Trend(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (delta float64, ok bool, err error) {
	repo := readings.New(s.db)
	now, ok, err := repo.Last(ctx, id, m)
	if err != nil || !ok {
		return 0, false, err
	}
	then, ok, err := repo.Before(ctx, id, m, since)
	if err != nil || !ok {
		return 0, false, err
	}
	// One row on both ends is a value that has not moved since: flat, not
	// unknown.
	return now.Value.Num - then.Value.Num, true, nil
}

// Events returns at most limit journal events since a moment, newest
// first.
func (s *Store) Events(ctx context.Context, since time.Time, limit int) ([]model.Event, error) {
	return events.New(s.db).Since(ctx, since, limit)
}
