//go:build integration

package application

import (
	"context"
	"errors"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
)

// writeConfig puts a YAML file into dir and returns its path.
func writeConfig(t *testing.T, dir, yaml string) string {
	t.Helper()
	path := filepath.Join(dir, "domovoi.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// pngConfig is a config with one pngfile display writing frame.png into dir.
func pngConfig(t *testing.T, dir, extra string) string {
	t.Helper()
	return writeConfig(t, dir, extra+"displays:\n  - kind: pngfile\n    path: "+filepath.Join(dir, "frame.png")+"\n")
}

// failing is a module whose Start reports a fatal failure shortly after.
type failing struct {
	container.Passive
	c *container.Container
}

func (f *failing) Name() string { return "failing" }

func (f *failing) Register(c *container.Container) error { f.c = c; return nil }

func (f *failing) Start(context.Context) error {
	go func() {
		time.Sleep(20 * time.Millisecond)
		f.c.Fail(errors.New("hardware gone"))
	}()
	return nil
}

func TestRun(t *testing.T) {
	t.Run("once renders a frame with no sources", func(t *testing.T) {
		dir := t.TempDir()
		path := pngConfig(t, dir, "language: ru\n")
		err := Run(t.Context(), Options{ConfigPath: path, Once: true, Lookup: config.MapLookup(nil), LogOutput: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(filepath.Join(dir, "frame.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 480 {
			t.Errorf("frame is %v", img.Bounds())
		}
	})
	t.Run("check builds everything and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		path := pngConfig(t, dir, "")
		if err := Run(t.Context(), Options{ConfigPath: path, Check: true, Lookup: config.MapLookup(nil), LogOutput: io.Discard}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "frame.png")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("check wrote a frame: %v", err)
		}
	})
	t.Run("runs until cancelled", func(t *testing.T) {
		path := pngConfig(t, t.TempDir(), "")
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		if err := Run(ctx, Options{ConfigPath: path, Lookup: config.MapLookup(nil), LogOutput: io.Discard}); err != nil {
			t.Fatalf("Run = %v, want nil on cancellation", err)
		}
	})
	t.Run("a module failure stops the run with its cause", func(t *testing.T) {
		path := pngConfig(t, t.TempDir(), "")
		mods := append(enabled(), &failing{})
		err := Run(t.Context(), Options{ConfigPath: path, Lookup: config.MapLookup(nil), LogOutput: io.Discard, Modules: mods})
		if err == nil || err.Error() != "hardware gone" {
			t.Fatalf("Run = %v, want the module's cause", err)
		}
	})
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "missing config", yaml: "", wantErr: "no such file"},
		{name: "unknown display kind", yaml: "displays:\n  - kind: hologram\n", wantErr: `unknown kind "hologram" (known: pngfile)`},
		{name: "unknown source kind", yaml: "sources:\n  - kind: zwave\ndisplays:\n  - kind: pngfile\n    path: /tmp/x.png\n", wantErr: `unknown kind "zwave"`},
		{name: "unknown scene", yaml: "displays:\n  - kind: pngfile\n    path: /tmp/x.png\n    scenes: [galaxy]\n", wantErr: `scene galaxy`},
		{name: "unknown language", yaml: "language: xx\ndisplays:\n  - kind: pngfile\n    path: /tmp/x.png\n", wantErr: `language "xx"`},
		{name: "unresolved secret", yaml: "sources:\n  - kind: tuya\n    access_id: ${NOPE}\n    access_key: ${NOPE}\ndisplays:\n  - kind: pngfile\n    path: /tmp/x.png\n", wantErr: "unresolved secrets: NOPE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing.yaml")
			if tt.yaml != "" {
				path = writeConfig(t, t.TempDir(), tt.yaml)
			}
			err := Run(t.Context(), Options{ConfigPath: path, Check: true, Lookup: config.MapLookup(nil), LogOutput: io.Discard})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Run = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// assembling records whether the application called its Assemble hook.
type assembling struct {
	container.Passive
	assembled bool
}

func (a *assembling) Name() string                        { return "assembling" }
func (a *assembling) Register(*container.Container) error { return nil }
func (a *assembling) Assemble(c *container.Container, cfg config.Config) error {
	if _, err := c.Core.Get(); err != nil { // the core is wired by now
		return err
	}
	a.assembled = len(cfg.Displays) == 1
	return nil
}

func TestRunAssembler(t *testing.T) {
	path := pngConfig(t, t.TempDir(), "")
	a := &assembling{}
	mods := append(enabled(), a)
	if err := Run(t.Context(), Options{ConfigPath: path, Check: true, Lookup: config.MapLookup(nil), LogOutput: io.Discard, Modules: mods}); err != nil {
		t.Fatal(err)
	}
	if !a.assembled {
		t.Error("Assemble was not called after the built-in wiring")
	}
}
