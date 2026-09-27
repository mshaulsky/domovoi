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

func TestAqaraRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "key", yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n" + base},
		{name: "own endpoint", yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    endpoint: http://localhost:9/mcp\n" + base},
		{name: "account", yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: ru\n" + base},
		{name: "the password itself is refused", yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    password_md5: ${KEY}\n    region: RU\n" + base, wantErr: "password MD5 must be 32 hex digits"},
		{name: "key and account", yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    username: home@example.com\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: RU\n" + base},
		{name: "key or account required", yaml: "sources:\n  - kind: aqara\n" + base, wantErr: "api_key or an account (username, password_md5, region) is required"},
		{name: "password without username", yaml: "sources:\n  - kind: aqara\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n" + base, wantErr: "username is required with password_md5"},
		{name: "account without password", yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    region: RU\n" + base, wantErr: "password_md5 is required with username"},
		{name: "account without region", yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n" + base, wantErr: "region is required with username"},
		{name: "unknown region", yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: mars\n" + base, wantErr: `region "mars" is not one of EU, RU, US, CN, KR, SG`},
		{name: "unknown option", yaml: "sources:\n  - kind: aqara\n    apikey: ${KEY}\n" + base, wantErr: "field apikey not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			if err := (Aqara{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Sources.Get("aqara")
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
			if src.Name() != "aqara" {
				t.Errorf("source name = %q", src.Name())
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := Aqara{}
		if m.Name() != "aqara" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}

func TestWeatherRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "valid", yaml: "timezone: Asia/Almaty\nsources:\n  - kind: weather\n    latitude: 43.25\n    longitude: 76.9\n" + base},
		{name: "another endpoint", yaml: "sources:\n  - kind: weather\n    latitude: 43.25\n    longitude: 76.9\n    url: http://127.0.0.1:1/f\n" + base},
		{name: "coordinates required", yaml: "sources:\n  - kind: weather\n    latitude: 43.25\n" + base, wantErr: "latitude and longitude are required"},
		{name: "latitude out of range", yaml: "sources:\n  - kind: weather\n    latitude: 95\n    longitude: 76.9\n" + base, wantErr: "latitude 95 out of range"},
		{name: "unknown option", yaml: "sources:\n  - kind: weather\n    lat: 43.25\n    longitude: 76.9\n" + base, wantErr: "field lat not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			cfg := parse(t, tt.yaml)
			c.Config.Set(&cfg)
			if err := (Weather{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Sources.Get("weather")
			if err != nil {
				t.Fatal(err)
			}
			src, err := ctor(cfg.Sources[0])
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if src.Name() != "weather" {
				t.Errorf("source name = %q", src.Name())
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := Weather{}
		if m.Name() != "weather" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
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
	c.Config.Set(&config.Config{})
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
