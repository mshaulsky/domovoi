package modules

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/metrics"
)

const base = "displays:\n  - kind: pngfile\n"

// parse builds a config from YAML with the given source and display
// sections appended.
func parse(t *testing.T, yaml string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml), config.MapLookup(map[string]string{"ID": "id", "KEY": "key"}))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newContainer() *container.Container {
	c := container.New(nil)
	c.Logger.Set(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.Metrics.Set(metrics.New())
	return c
}

func TestTuyaRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "valid", yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n    region: eu\n" + base},
		{name: "default region", yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n" + base},
		{name: "credentials required", yaml: "sources:\n  - kind: tuya\n    region: eu\n" + base, wantErr: "access_id and access_key are required"},
		{name: "unknown region", yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n    region: mars\n" + base, wantErr: `unknown region "mars"`},
		{name: "unknown option", yaml: "sources:\n  - kind: tuya\n    acess_id: ${ID}\n    access_key: ${KEY}\n" + base, wantErr: "field acess_id not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			if err := (Tuya{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Sources.Get("tuya")
			if err != nil {
				t.Fatal(err)
			}
			src, err := ctor(parse(t, tt.yaml).Sources[0])
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if src.Name() != "tuya" {
				t.Errorf("source name = %q", src.Name())
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := Tuya{}
		if m.Name() != "tuya" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}

func TestPNGFileRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "valid", yaml: "displays:\n  - kind: pngfile\n    path: /tmp/frame.png\n    width: 400\n    height: 300\n    mono: true\n"},
		{name: "path required", yaml: base, wantErr: "path is required"},
		{name: "unknown option", yaml: "displays:\n  - kind: pngfile\n    file: /tmp/frame.png\n", wantErr: "field file not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			if err := (PNGFile{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Displays.Get("pngfile")
			if err != nil {
				t.Fatal(err)
			}
			d, err := ctor(parse(t, tt.yaml).Displays[0])
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s := d.Surface(); s.Bounds.Dx() != 400 || len(s.Palette) != 2 {
				t.Errorf("surface = %+v", s)
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := PNGFile{}
		if m.Name() != "pngfile" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}

func TestScenesRegister(t *testing.T) {
	c := newContainer()
	m := Scenes{}
	if err := m.Register(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Scenes.Get("overview"); err != nil {
		t.Errorf("overview not registered: %v", err)
	}
	if err := m.Register(c); err == nil {
		t.Error("registering twice should fail")
	}
	if m.Name() != "scenes" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
		t.Error("passive lifecycle misbehaves")
	}
}

func TestCoreRegister(t *testing.T) {
	c := newContainer()
	m := &Core{}
	if err := m.Register(c); err != nil {
		t.Fatal(err)
	}
	if m.Name() != "core" {
		t.Errorf("Name = %q", m.Name())
	}
	if err := m.Start(t.Context()); err == nil {
		t.Error("Start before the core is built should fail")
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Errorf("Stop before build = %v", err)
	}
	core, err := c.Core.Get()
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.State.Get()
	if core == nil || err != nil || st == nil {
		t.Errorf("core or state not built: %v", err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Errorf("Stop = %v", err)
	}
}
