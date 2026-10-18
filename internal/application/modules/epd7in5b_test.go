package modules

import (
	"strings"
	"testing"
)

func TestEPD7in5bRegister(t *testing.T) {
	const head = "displays:\n  - kind: epd7in5b\n"
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "defaults",
			yaml: head,
		},
		{
			name: "every key",
			yaml: head + "    spi: /dev/spidev0.0\n" +
				"    pins: {rst: GPIO5, dc: GPIO6, busy: GPIO13, power: none}\n" +
				"    rotation: 180\n    border: red\n    timeout: 2m\n",
		},
		{
			name: "one pin overridden",
			yaml: head + "    pins: {busy: GPIO23}\n",
		},
		{
			name:    "rotation 90",
			yaml:    head + "    rotation: 90\n",
			wantErr: "rotation 90 is not 0 or 180",
		},
		{
			name:    "unknown border",
			yaml:    head + "    border: pink\n",
			wantErr: `border "pink" is not white, black or red`,
		},
		{
			name:    "empty reset pin",
			yaml:    head + "    pins: {rst: \"\"}\n",
			wantErr: "pins rst, dc and busy are required",
		},
		{
			name:    "negative timeout",
			yaml:    head + "    timeout: -1s\n",
			wantErr: "must not be negative",
		},
		{
			name:    "unknown option",
			yaml:    head + "    speed: 8MHz\n",
			wantErr: "field speed not found",
		},
		{
			name:    "unknown pin",
			yaml:    head + "    pins: {cs: GPIO8}\n",
			wantErr: "field cs not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			c.Check = true // nothing is opened
			if err := (EPD7in5b{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Displays.Get("epd7in5b")
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
			if s := d.Surface(); s.Bounds.Dx() != 800 || s.Bounds.Dy() != 480 || len(s.Palette) != 3 || !s.Partial ||
				s.WindowAlign != 8 {
				t.Errorf("surface = %+v", s)
			}
			if err := d.Close(t.Context()); err != nil {
				t.Errorf("Close = %v", err)
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := EPD7in5b{}
		if m.Name() != "epd7in5b" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}
