//go:build integration

package pngfile

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mshaulsky/domovoi/internal/display"
)

// TestDisplayShowWritesFile is the happy path on a real filesystem: the
// frame lands at the configured path, decodes back to the same pixels, and
// no temporary file is left behind.
func TestDisplayShowWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame.png")
	d, err := New(Config{Path: path, Width: 4, Height: 2})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewPaletted(image.Rect(0, 0, 4, 2), display.ThreeColour())
	img.SetColorIndex(1, 0, 1) // black
	img.SetColorIndex(2, 1, 2) // red

	for i := range 2 { // twice: the second Show replaces the first file
		if err := d.Show(t.Context(), display.Frame{Image: img, Mode: display.ModeFull}); err != nil {
			t.Fatalf("Show #%d: %v", i+1, err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Errorf("bounds = %v, want %v", decoded.Bounds(), img.Bounds())
	}
	if r, g, b, _ := decoded.At(2, 1).RGBA(); r>>8 != 0xFF || g != 0 || b != 0 {
		t.Errorf("pixel (2,1) = %v, want red", decoded.At(2, 1))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the frame", len(entries))
	}
}
