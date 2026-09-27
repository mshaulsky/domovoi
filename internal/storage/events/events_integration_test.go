//go:build integration

package events_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/storage/events"
	"github.com/mshaulsky/domovoi/internal/storage/sqlite"
)

var epoch = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "domovoi.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sameEvent(got, want model.Event) bool {
	return got.Kind == want.Kind && got.Device == want.Device && got.Detail == want.Detail && got.At.Equal(want.At)
}

func TestRepositoryWrite(t *testing.T) {
	db := openDB(t)
	r := events.New(db)
	evs := []model.Event{
		{Device: "aqara:lock", Kind: model.EventUnlocked, Detail: "front", At: epoch},
		{Kind: model.EventAlertRaised, Detail: "leak", At: epoch}, // no device
	}
	if err := r.Write(t.Context(), evs); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM events`).Scan(&rows); err != nil || rows != 2 {
		t.Errorf("rows = %d, %v", rows, err)
	}
	if err := r.Write(t.Context(), nil); err != nil {
		t.Errorf("empty write = %v", err)
	}
}

func TestRepositorySince(t *testing.T) {
	db := openDB(t)
	r := events.New(db)
	evs := []model.Event{
		{Device: "tuya:hall", Kind: model.EventOnline, At: epoch.Add(-2 * time.Hour)},
		{Device: "aqara:lock", Kind: model.EventUnlocked, At: epoch.Add(-time.Hour)},
		{Device: "aqara:lock", Kind: model.EventLocked, At: epoch},
	}
	if err := r.Write(t.Context(), evs); err != nil {
		t.Fatal(err)
	}
	got, err := r.Since(t.Context(), epoch.Add(-90*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !sameEvent(got[0], evs[2]) || !sameEvent(got[1], evs[1]) {
		t.Errorf("Since = %+v; want the two newest, newest first", got)
	}
	if got, err := r.Since(t.Context(), epoch.Add(-3*time.Hour), 1); err != nil || len(got) != 1 || !sameEvent(got[0], evs[2]) {
		t.Errorf("limit 1 = %+v, %v", got, err)
	}
}

func TestRepositoryPrune(t *testing.T) {
	db := openDB(t)
	r := events.New(db)
	evs := []model.Event{
		{Device: "tuya:hall", Kind: model.EventOnline, At: epoch.Add(-48 * time.Hour)},
		{Device: "tuya:hall", Kind: model.EventOffline, At: epoch},
	}
	if err := r.Write(t.Context(), evs); err != nil {
		t.Fatal(err)
	}
	n, err := r.Prune(t.Context(), epoch.Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("Prune = %d, %v; want 1", n, err)
	}
	if got, err := r.Since(t.Context(), epoch.Add(-72*time.Hour), 10); err != nil || len(got) != 1 {
		t.Errorf("after prune = %+v, %v", got, err)
	}
}
