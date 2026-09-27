//go:build integration

package modules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mshaulsky/domovoi/internal/config"
)

func TestStorageModule(t *testing.T) {
	if got := (&Storage{}).Name(); got != "storage" {
		t.Errorf("Name = %q", got)
	}
	tests := []struct {
		name     string
		check    bool
		missing  bool // the configured directory does not exist
		wantErr  string
		wantFile bool
	}{
		{name: "opens and migrates the file", wantFile: true},
		{name: "check leaves the file alone", check: true},
		{name: "check rejects a missing directory", check: true, missing: true, wantErr: "database directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.missing {
				dir = filepath.Join(dir, "nope")
			}
			path := filepath.Join(dir, "domovoi.db")
			c := newContainer()
			c.Check = tt.check
			c.Config.Set(&config.Config{Storage: config.StorageSection{Path: path}})
			m := &Storage{}
			if err := m.Register(c); err != nil {
				t.Fatal(err)
			}
			if err := m.Stop(t.Context()); err != nil {
				t.Errorf("Stop before use = %v", err)
			}
			_, err := c.Store.Get()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Get = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.db == nil {
				t.Fatal("Get did not open the database")
			}
			if _, err := os.Stat(path); (err == nil) != tt.wantFile || (err != nil && !errors.Is(err, os.ErrNotExist)) {
				t.Errorf("file exists = %v (%v), want %v", err == nil, err, tt.wantFile)
			}
			if err := m.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := m.Stop(t.Context()); err != nil {
				t.Errorf("Stop = %v", err)
			}
			if err := m.db.Ping(); err == nil {
				t.Error("database still open after Stop")
			}
		})
	}
}
