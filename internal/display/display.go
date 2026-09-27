package display

import (
	"context"
	"image"
	"image/color"
	"time"
)

// Surface describes what a display can show. Scenes are laid out against it.
type Surface struct {
	Bounds      image.Rectangle
	Palette     color.Palette // e.g. white, black, red; mono panels: white, black
	Partial     bool          // supports windowed updates
	FullRefresh time.Duration // how long a full refresh takes, for scheduling and logs
}

// Frame is a rendered scene in the display's palette. Window is the region
// to update for ModePartial and ignored otherwise.
type Frame struct {
	Image  *image.Paletted
	Window image.Rectangle
	Mode   Mode
}

// Mode is how a frame reaches the glass.
type Mode int

// Display is a physical or virtual display. Show blocks for the panel's
// refresh and must return promptly once ctx is cancelled, leaving the glass
// in an unknown state the coordinator will redraw. A display serialises Show
// and Close itself: Close may be called while a cancelled Show is still
// unwinding and must wait for it before putting the panel to sleep.
type Display interface {
	Surface() Surface
	Show(ctx context.Context, f Frame) error
	Close(ctx context.Context) error // sleep, flush, release
}

// Refresh modes.
const (
	ModeFull    Mode = iota // full waveform, every colour
	ModeFast                // shorter waveform, every colour
	ModePartial             // windowed, monochrome on our panel
)

// The colours a three-colour e-ink panel can show. Palettes are built from
// these so the theme can tell paper, ink and accent apart by value.
var (
	White = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	Black = color.RGBA{A: 0xFF}
	Red   = color.RGBA{R: 0xFF, A: 0xFF}
)

// ThreeColour is the palette of a black/white/red panel.
func ThreeColour() color.Palette {
	return color.Palette{White, Black, Red}
}

// Mono is the palette of a black/white panel.
func Mono() color.Palette {
	return color.Palette{White, Black}
}

// String names the mode for logs and metrics.
func (m Mode) String() string {
	switch m {
	case ModeFull:
		return "full"
	case ModeFast:
		return "fast"
	case ModePartial:
		return "partial"
	default:
		return "unknown"
	}
}
