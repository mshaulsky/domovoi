package pngfile

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"

	"github.com/mshaulsky/domovoi/internal/display"
)

// Display writes frames to a file.
type Display struct {
	cfg     Config
	surface display.Surface
	mu      sync.Mutex // Show and Close never overlap
}

// Config describes the file and the virtual panel it stands for.
type Config struct {
	Path   string
	Width  int  // defaults to DefaultWidth
	Height int  // defaults to DefaultHeight
	Mono   bool // black/white palette instead of black/white/red
}

// Defaults match the 7.5" panel so a PC preview is what the Pi will show.
const (
	DefaultWidth  = 800
	DefaultHeight = 480
)

// New validates the config and returns the display. Nothing is written
// until the first Show.
func New(cfg Config) (*Display, error) {
	if cfg.Path == "" {
		return nil, errors.New("pngfile: path is required")
	}
	if cfg.Width == 0 {
		cfg.Width = DefaultWidth
	}
	if cfg.Height == 0 {
		cfg.Height = DefaultHeight
	}
	if cfg.Width < 0 || cfg.Height < 0 {
		return nil, fmt.Errorf("pngfile: size %dx%d must be positive", cfg.Width, cfg.Height)
	}
	palette := display.ThreeColour()
	if cfg.Mono {
		palette = display.Mono()
	}
	return &Display{
		cfg: cfg,
		surface: display.Surface{
			Bounds:  image.Rect(0, 0, cfg.Width, cfg.Height),
			Palette: palette,
		},
	}, nil
}

// Surface returns the virtual panel's geometry and palette.
func (d *Display) Surface() display.Surface {
	return d.surface
}

// Show writes the frame atomically: to a temporary file beside the target,
// then renamed over it, so a reader never sees a half-written PNG.
func (d *Display) Show(_ context.Context, f display.Frame) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if f.Image == nil {
		return errors.New("pngfile: frame has no image")
	}
	dir := filepath.Dir(d.cfg.Path)
	tmp, err := os.CreateTemp(dir, ".frame-*.png")
	if err != nil {
		return fmt.Errorf("pngfile: create temporary file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := png.Encode(tmp, f.Image); err != nil {
		tmp.Close()
		return fmt.Errorf("pngfile: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("pngfile: close temporary file: %w", err)
	}
	if err := os.Rename(tmp.Name(), d.cfg.Path); err != nil {
		return fmt.Errorf("pngfile: replace %s: %w", d.cfg.Path, err)
	}
	return nil
}

// Close waits for a Show in flight; the last frame stays on disk.
func (d *Display) Close(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return nil
}
