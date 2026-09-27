//go:build integration

package settings_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/storage/settings"
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

func TestRepositoryGet(t *testing.T) {
	db := openDB(t)
	r := settings.New(db)
	if _, ok, err := r.Get(t.Context(), "language"); ok || err != nil {
		t.Errorf("unset key: ok=%t err=%v", ok, err)
	}
	if err := r.Set(t.Context(), "language", "ru", epoch); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := r.Get(t.Context(), "language"); !ok || err != nil || v != "ru" {
		t.Errorf("Get = %q %t %v", v, ok, err)
	}
}

func TestRepositorySet(t *testing.T) {
	db := openDB(t)
	r := settings.New(db)
	if err := r.Set(t.Context(), "language", "ru", epoch); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(t.Context(), "language", "en", epoch.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var v string
	var at int64
	if err := db.QueryRowContext(t.Context(), `SELECT value, updated_at FROM settings WHERE key = 'language'`).Scan(&v, &at); err != nil {
		t.Fatal(err)
	}
	if v != "en" || at != epoch.Add(time.Minute).Unix() {
		t.Errorf("row = %q at %d; want en at %d", v, at, epoch.Add(time.Minute).Unix())
	}
}

func TestRepositoryDelete(t *testing.T) {
	db := openDB(t)
	r := settings.New(db)
	if err := r.Set(t.Context(), "language", "ru", epoch); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(t.Context(), "language"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(t.Context(), "language"); ok {
		t.Error("deleted key still set")
	}
	if err := r.Delete(t.Context(), "language"); err != nil {
		t.Errorf("deleting an absent key = %v", err)
	}
}

func TestRepositoryAll(t *testing.T) {
	db := openDB(t)
	r := settings.New(db)
	if all, err := r.All(t.Context()); err != nil || len(all) != 0 {
		t.Errorf("empty table: %v, %v", all, err)
	}
	for k, v := range map[string]string{"language": "en", "tick": "5m"} {
		if err := r.Set(t.Context(), k, v, epoch); err != nil {
			t.Fatal(err)
		}
	}
	if all, err := r.All(t.Context()); err != nil || len(all) != 2 || all["language"] != "en" || all["tick"] != "5m" {
		t.Errorf("All = %v %v", all, err)
	}
}
