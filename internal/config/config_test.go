package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimal = `
displays:
  - kind: pngfile
`

func checkErr(t *testing.T, err error, wantErr string) bool {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return false
	}
	if err == nil {
		t.Fatalf("got no error, want one containing %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("error %q does not contain %q", err, wantErr)
	}
	return true
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte(minimal), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("displays: nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "valid file", path: good},
		{name: "missing file", path: filepath.Join(dir, "none.yaml"), wantErr: "none.yaml"},
		{name: "invalid content", path: bad, wantErr: "bad.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.path, MapLookup(nil))
			if checkErr(t, err, tt.wantErr) {
				return
			}
			if len(cfg.Displays) != 1 {
				t.Errorf("Load returned %d displays, want 1", len(cfg.Displays))
			}
		})
	}
}

func TestParse(t *testing.T) {
	lookup := MapLookup(map[string]string{"KEY": "s3cret", "ID": "id-1"})
	tests := []struct {
		name    string
		yaml    string
		lookup  Lookup
		want    func(t *testing.T, cfg Config)
		wantErr string
	}{
		{
			name: "defaults",
			yaml: minimal,
			want: func(t *testing.T, cfg Config) {
				if cfg.Language != "en" || cfg.Timezone != time.Local {
					t.Errorf("language/timezone = %q/%v, want en/Local", cfg.Language, cfg.Timezone)
				}
				d := cfg.Displays[0]
				if d.Name != "pngfile" || d.Tick != 5*time.Minute || d.FullEvery != time.Hour || d.Language != "en" {
					t.Errorf("display defaults = %+v", d)
				}
				if len(d.Scenes) != 1 || d.Scenes[0] != "overview" {
					t.Errorf("scenes = %v, want [overview]", d.Scenes)
				}
				if err := d.Options.Decode(&struct{}{}); err != nil {
					t.Errorf("options should be empty: %v", err)
				}
			},
		},
		{
			name: "full document with secrets",
			yaml: `
timezone: Asia/Almaty
language: ru
sources:
  - kind: tuya
    interval: 30s
    access_id: ${ID}
    access_key: ${KEY}
    region: eu
  - kind: tuya
    name: tuya-office
    stale_after: 20m
displays:
  - kind: epd7in5b
    name: hall
    scenes: [overview, climate]
    tick: 1m
    full_every: 30m
    language: en
    spi: /dev/spidev0.0
`,
			lookup: lookup,
			want: func(t *testing.T, cfg Config) {
				if cfg.Timezone.String() != "Asia/Almaty" || cfg.Language != "ru" {
					t.Errorf("timezone/language = %v/%q", cfg.Timezone, cfg.Language)
				}
				s := cfg.Sources[0]
				if s.Name != "tuya" || s.Interval != 30*time.Second || s.StaleAfter != 5*time.Minute {
					t.Errorf("source[0] = %+v", s)
				}
				var opts struct {
					AccessID  string `yaml:"access_id"`
					AccessKey Secret `yaml:"access_key"`
					Region    string `yaml:"region"`
				}
				if err := s.Options.Decode(&opts); err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if opts.AccessID != "id-1" || opts.AccessKey.Reveal() != "s3cret" || opts.Region != "eu" {
					t.Errorf("options = %+v, secret %q", opts, opts.AccessKey.Reveal())
				}
				if s2 := cfg.Sources[1]; s2.Name != "tuya-office" || s2.Interval != time.Minute || s2.StaleAfter != 20*time.Minute {
					t.Errorf("source[1] = %+v", s2)
				}
				d := cfg.Displays[0]
				if d.Name != "hall" || d.Tick != time.Minute || d.FullEvery != 30*time.Minute || d.Language != "en" || len(d.Scenes) != 2 {
					t.Errorf("display = %+v", d)
				}
			},
		},
		{
			name:    "unresolved secrets listed",
			yaml:    "sources:\n  - kind: tuya\n    a: ${ONE}\n    b: ${TWO}-${ONE}\n" + minimal,
			wantErr: "unresolved secrets: ONE, TWO",
		},
		{
			name:   "placeholder inside a longer string",
			yaml:   "sources:\n  - kind: tuya\n    url: https://${ID}.example/${KEY}\n" + minimal,
			lookup: lookup,
			want: func(t *testing.T, cfg Config) {
				var o struct {
					URL string `yaml:"url"`
				}
				if err := cfg.Sources[0].Options.Decode(&o); err != nil {
					t.Fatal(err)
				}
				if o.URL != "https://id-1.example/s3cret" {
					t.Errorf("url = %q", o.URL)
				}
			},
		},
		{
			name:   "secret with yaml punctuation survives",
			yaml:   "sources:\n  - kind: tuya\n    key: ${KEY}\n" + minimal,
			lookup: MapLookup(map[string]string{"KEY": "a: b #c {d} 'e'"}),
			want: func(t *testing.T, cfg Config) {
				var o struct {
					Key string `yaml:"key"`
				}
				if err := cfg.Sources[0].Options.Decode(&o); err != nil {
					t.Fatal(err)
				}
				if o.Key != "a: b #c {d} 'e'" {
					t.Errorf("key = %q", o.Key)
				}
			},
		},
		{name: "unknown top-level key", yaml: "colour: red\n" + minimal, wantErr: "field colour not found"},
		{name: "no displays", yaml: "sources:\n  - kind: tuya\n", wantErr: "at least one display"},
		{name: "missing kind", yaml: "displays:\n  - name: x\n", wantErr: "displays[0]: kind is required"},
		{name: "duplicate names", yaml: "displays:\n  - kind: pngfile\n  - kind: pngfile\n", wantErr: `duplicate name "pngfile"`},
		{name: "name with colon", yaml: "sources:\n  - kind: tuya\n    name: a:b\n" + minimal, wantErr: `name "a:b" must match`},
		{name: "negative interval", yaml: "sources:\n  - kind: tuya\n    interval: -1s\n" + minimal, wantErr: "interval must be positive"},
		{name: "bad duration", yaml: "sources:\n  - kind: tuya\n    interval: soon\n" + minimal, wantErr: "sources[0]"},
		{name: "bad timezone", yaml: "timezone: Mars/Olympus\n" + minimal, wantErr: `timezone "Mars/Olympus"`},
		{name: "section is not a mapping", yaml: "displays:\n  - pngfile\n", wantErr: "expected a mapping, got a scalar"},
		{name: "empty scene name", yaml: "displays:\n  - kind: pngfile\n    scenes: ['']\n", wantErr: "empty scene name"},
		{name: "invalid yaml", yaml: "displays: [", wantErr: "parse yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lk := tt.lookup
			if lk == nil {
				lk = MapLookup(nil)
			}
			cfg, err := Parse([]byte(tt.yaml), lk)
			if checkErr(t, err, tt.wantErr) {
				return
			}
			tt.want(t, cfg)
		})
	}
}

func TestOptionsDecode(t *testing.T) {
	type opts struct {
		Path  string `yaml:"path"`
		Width int    `yaml:"width"`
	}
	tests := []struct {
		name    string
		yaml    string
		want    opts
		wantErr string
	}{
		{name: "known keys", yaml: "displays:\n  - kind: pngfile\n    path: /tmp/f.png\n    width: 800\n", want: opts{Path: "/tmp/f.png", Width: 800}},
		{name: "no options", yaml: minimal, want: opts{}},
		{name: "unknown key", yaml: "displays:\n  - kind: pngfile\n    pth: /tmp/f.png\n", wantErr: "field pth not found"},
		{name: "type mismatch", yaml: "displays:\n  - kind: pngfile\n    width: wide\n", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tt.yaml), MapLookup(nil))
			if err != nil {
				t.Fatal(err)
			}
			var got opts
			err = cfg.Displays[0].Options.Decode(&got)
			if checkErr(t, err, tt.wantErr) {
				return
			}
			if got != tt.want {
				t.Errorf("Decode = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultLookup(t *testing.T) {
	creds := t.TempDir()
	secrets := t.TempDir()
	if err := os.WriteFile(filepath.Join(creds, "A"), []byte("from-creds\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "A"), []byte("from-secrets"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "B"), []byte("b-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"A": "from-env", "B": "from-env", "C": "c-env"}
	getenv := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	tests := []struct {
		name     string
		credsDir string
		key      string
		want     string
		wantOK   bool
	}{
		{name: "credentials first, newline trimmed", credsDir: creds, key: "A", want: "from-creds", wantOK: true},
		{name: "secrets second", credsDir: creds, key: "B", want: "b-secret", wantOK: true},
		{name: "environment last", credsDir: creds, key: "C", want: "c-env", wantOK: true},
		{name: "no credentials directory", credsDir: "", key: "A", want: "from-secrets", wantOK: true},
		{name: "unknown", credsDir: creds, key: "D", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := lookupWith(tt.credsDir, secrets, getenv)(tt.key)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("lookup(%q) = %q, %t; want %q, %t", tt.key, got, ok, tt.want, tt.wantOK)
			}
		})
	}
	t.Run("wired to the environment", func(t *testing.T) {
		t.Setenv("CREDENTIALS_DIRECTORY", creds)
		t.Setenv("DOMOVOI_TEST_SECRET", "env-value")
		if got, ok := DefaultLookup()("A"); !ok || got != "from-creds" {
			t.Errorf("A = %q, %t", got, ok)
		}
		if got, ok := DefaultLookup()("DOMOVOI_TEST_SECRET"); !ok || got != "env-value" {
			t.Errorf("DOMOVOI_TEST_SECRET = %q, %t", got, ok)
		}
	})
}

func TestMapLookup(t *testing.T) {
	lk := MapLookup(map[string]string{"A": "1"})
	if got, ok := lk("A"); !ok || got != "1" {
		t.Errorf("A = %q, %t", got, ok)
	}
	if _, ok := lk("B"); ok {
		t.Error("B should be unknown")
	}
}
