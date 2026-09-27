//go:build integration

package sqlite

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domovoi.db")
	db, err := Open(t.Context(), path, quiet)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v; want wal", mode, err)
	}
	var tables int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'
		AND name IN ('devices', 'series', 'readings', 'events', 'alerts', 'settings')`).Scan(&tables); err != nil || tables != 6 {
		t.Errorf("tables = %d, %v; want 6", tables, err)
	}
	// Opening again is a no-op migration.
	again, err := Open(t.Context(), path, quiet)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	again.Close()
	// The in-memory database migrates the same way: what -check runs.
	mem, err := Open(t.Context(), Memory, quiet)
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer mem.Close()
	if err := mem.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil || tables < 6 {
		t.Errorf("memory tables = %d, %v; want at least 6", tables, err)
	}
	// A file from a newer binary is refused.
	if _, err := db.ExecContext(t.Context(), `INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (999, 1, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path, quiet); !errors.Is(err, ErrNewer) {
		t.Errorf("newer schema: err = %v, want ErrNewer", err)
	}
}
