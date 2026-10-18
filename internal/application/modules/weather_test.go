package modules

import (
	"strings"
	"testing"
)

func TestWeatherRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid",
			yaml: "timezone: Asia/Almaty\nsources:\n  - kind: weather\n    latitude: 43.25\n" +
				"    longitude: 76.9\n" + base,
		},
		{
			name: "another endpoint",
			yaml: "sources:\n  - kind: weather\n    latitude: 43.25\n    longitude: 76.9\n" +
				"    url: http://127.0.0.1:1/f\n" + base,
		},
		{
			name:    "coordinates required",
			yaml:    "sources:\n  - kind: weather\n    latitude: 43.25\n" + base,
			wantErr: "latitude and longitude are required",
		},
		{
			name:    "latitude out of range",
			yaml:    "sources:\n  - kind: weather\n    latitude: 95\n    longitude: 76.9\n" + base,
			wantErr: "latitude 95 out of range",
		},
		{
			name:    "unknown option",
			yaml:    "sources:\n  - kind: weather\n    lat: 43.25\n    longitude: 76.9\n" + base,
			wantErr: "field lat not found",
		},
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
