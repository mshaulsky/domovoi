package modules

import (
	"strings"
	"testing"
)

func TestPNGFileRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid",
			yaml: "displays:\n  - kind: pngfile\n    path: /tmp/frame.png\n    width: 400\n    height: 300\n" +
				"    mono: true\n",
		},
		{
			name:    "path required",
			yaml:    base,
			wantErr: "path is required",
		},
		{
			name:    "unknown option",
			yaml:    "displays:\n  - kind: pngfile\n    file: /tmp/frame.png\n",
			wantErr: "field file not found",
		},
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
