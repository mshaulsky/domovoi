//go:build integration

package devices_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/storage/devices"
	"github.com/mshaulsky/domovoi/internal/storage/sqlite"
)

var (
	epoch = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	hall  = model.Device{ID: "tuya:hall", Name: "Зал", Kind: model.KindClimateSensor}
	lock  = model.Device{ID: "aqara:lock", Name: "Door", Room: "Hallway", Kind: model.KindLock}
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "domovoi.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRepositoryUpsert(t *testing.T) {
	db := openDB(t)
	r := devices.New(db)
	if err := r.Upsert(t.Context(), []model.Device{hall, lock}, epoch); err != nil {
		t.Fatal(err)
	}
	renamed := hall
	renamed.Name, renamed.Room = "Living room", "Ground floor"
	if err := r.Upsert(t.Context(), []model.Device{renamed}, epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var name, room, source string
	var first, last int64
	if err := db.QueryRowContext(t.Context(), `SELECT name, room, source, first_seen, last_seen FROM devices WHERE id = ?`, string(hall.ID)).
		Scan(&name, &room, &source, &first, &last); err != nil {
		t.Fatal(err)
	}
	if name != "Living room" || room != "Ground floor" || source != "tuya" || first != epoch.Unix() || last != epoch.Add(time.Hour).Unix() {
		t.Errorf("row = %s %s %s first %d last %d", name, room, source, first, last)
	}
	if err := r.Upsert(t.Context(), nil, epoch); err != nil {
		t.Errorf("empty upsert = %v", err)
	}
}

func TestRepositoryPrune(t *testing.T) {
	db := openDB(t)
	r := devices.New(db)
	ghost := model.Device{ID: "aqara:ghost", Name: "Old lock", Kind: model.KindLock}
	silent := model.Device{ID: "tuya:silent", Name: "Silent", Kind: model.KindClimateSensor}
	if err := r.Upsert(t.Context(), []model.Device{ghost, silent}, epoch.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(t.Context(), []model.Device{hall}, epoch); err != nil {
		t.Fatal(err)
	}
	// The silent device still has a series: history keeps it.
	if _, err := db.ExecContext(t.Context(), `INSERT INTO series (device, metric) VALUES (?, ?)`, string(silent.ID), string(model.Temperature)); err != nil {
		t.Fatal(err)
	}
	n, err := r.Prune(t.Context(), epoch.Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("Prune = %d, %v; want 1", n, err)
	}
	got, err := r.All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != hall || got[1] != silent {
		t.Errorf("All after prune = %+v, want hall and silent", got)
	}
}

func TestRepositoryAll(t *testing.T) {
	db := openDB(t)
	r := devices.New(db)
	if got, err := r.All(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("empty table: %v, %v", got, err)
	}
	if err := r.Upsert(t.Context(), []model.Device{hall, lock}, epoch); err != nil {
		t.Fatal(err)
	}
	got, err := r.All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != lock || got[1] != hall { // by ID: aqara before tuya
		t.Errorf("All = %+v", got)
	}
}
