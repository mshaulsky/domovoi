package pngfile

import (
	"image"
	"strings"
	"testing"

	"github.com/mshaulsky/domovoi/internal/display"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    display.Surface
		wantErr string
	}{
		{
			name: "defaults",
			cfg:  Config{Path: "/tmp/frame.png"},
			want: display.Surface{Bounds: image.Rect(0, 0, 800, 480), Palette: display.ThreeColour()},
		},
		{
			name: "mono and custom size",
			cfg:  Config{Path: "/tmp/frame.png", Width: 250, Height: 122, Mono: true},
			want: display.Surface{Bounds: image.Rect(0, 0, 250, 122), Palette: display.Mono()},
		},
		{name: "path required", cfg: Config{}, wantErr: "path is required"},
		{name: "negative size", cfg: Config{Path: "x", Width: -1}, wantErr: "must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := New(tt.cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := d.Surface()
			if got.Bounds != tt.want.Bounds || len(got.Palette) != len(tt.want.Palette) || got.Partial {
				t.Errorf("Surface() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDisplayShow(t *testing.T) {
	d, err := New(Config{Path: t.TempDir() + "/frame.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Show(t.Context(), display.Frame{}); err == nil || !strings.Contains(err.Error(), "no image") {
		t.Errorf("Show(empty frame) = %v, want a 'no image' error", err)
	}
}

func TestDisplayClose(t *testing.T) {
	d, err := New(Config{Path: "/tmp/frame.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(t.Context()); err != nil {
		t.Errorf("Close() = %v", err)
	}
}
