package epd7in5b

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"time"

	epd "github.com/mshaulsky/waveshare-epd7in5b-v2"

	"github.com/mshaulsky/domovoi/internal/display"
)

// Display shows frames on the panel, powering it up for each one and
// putting it to sleep afterwards.
type Display struct {
	cfg     Config
	panel   Panel
	port    io.Closer // released by Close; nil when there is nothing to release
	log     *slog.Logger
	surface display.Surface
	// lock is a one-slot semaphore serialising Show and Close: a mutex in
	// all but name, a channel so that a Close waiting for a Show in flight
	// is durably blocked inside a synctest bubble.
	lock   chan struct{}
	awake  bool // a power-up was attempted and no Sleep has succeeded since
	closed bool
}

// Config tunes the display.
type Config struct {
	Timeout time.Duration // one Show from power-up to sleep; defaults to DefaultTimeout
}

const (
	// DefaultTimeout bounds one Show: a wash is two ~19 s refreshes plus two
	// power-ups, with room to spare.
	DefaultTimeout = 90 * time.Second

	// sleepTimeout is the display's own budget for putting the panel to sleep
	// after a Show, cancelled or not: an idle panel powers off in well under it.
	sleepTimeout = 5 * time.Second

	// fullRefresh is the measured full-colour refresh, 18.8 s.
	fullRefresh = 19 * time.Second

	// windowAlign is the column granularity of a partial window: the
	// controller addresses columns by the byte.
	windowAlign = 8
)

var _ display.Display = (*Display)(nil)

// New validates its collaborators and returns the display. The panel is not
// touched until the first Show. port is closed by Close and may be nil.
func New(cfg Config, panel Panel, port io.Closer, log *slog.Logger) (*Display, error) {
	if panel == nil {
		return nil, errors.New("epd7in5b: panel is required")
	}
	if log == nil {
		return nil, errors.New("epd7in5b: logger is required")
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("epd7in5b: timeout %s must not be negative", cfg.Timeout)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Display{
		cfg:   cfg,
		panel: panel,
		port:  port,
		log:   log,
		lock:  make(chan struct{}, 1),
		surface: display.Surface{
			Bounds:      image.Rect(0, 0, epd.Width, epd.Height),
			Palette:     display.ThreeColour(),
			Partial:     true,
			WindowAlign: windowAlign,
			FullRefresh: fullRefresh,
		},
	}, nil
}

// Surface returns the panel's geometry, palette and refresh abilities.
func (d *Display) Surface() display.Surface {
	return d.surface
}

// Show validates the frame, then powers the panel up for its mode, transfers
// it and puts the panel back to sleep — the last step even when an earlier
// one failed or ctx was cancelled. The whole cycle is bounded by
// Config.Timeout. A frame that reached the glass is a success even if the
// panel could not be put to sleep; that is logged and retried by Close.
func (d *Display) Show(ctx context.Context, f display.Frame) error {
	d.lock <- struct{}{}
	defer func() { <-d.lock }()
	if d.closed {
		return errors.New("epd7in5b: show after close")
	}
	if err := d.validate(f); err != nil {
		return err
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, d.cfg.Timeout)
	defer cancel()
	err := d.refresh(ctx, f)
	if serr := d.sleep(ctx); serr != nil {
		d.log.Warn("panel left awake", "mode", f.Mode, "err", serr)
	}
	if err != nil {
		return fmt.Errorf("epd7in5b: show %s: %w", f.Mode, err)
	}
	d.log.Debug("frame sent to the panel", "mode", f.Mode, "took", time.Since(start))
	return nil
}

// Close waits for a Show in flight, puts the panel to sleep if a failed
// Sleep left it awake, and releases the port. Calls after the first do
// nothing.
func (d *Display) Close(ctx context.Context) error {
	d.lock <- struct{}{}
	defer func() { <-d.lock }()
	if d.closed {
		return nil
	}
	d.closed = true
	var errs []error
	if d.awake {
		if err := d.panel.Sleep(ctx); err != nil {
			errs = append(errs, fmt.Errorf("epd7in5b: close: sleep: %w", err))
		} else {
			d.awake = false
		}
	}
	if d.port != nil {
		if err := d.port.Close(); err != nil {
			errs = append(errs, fmt.Errorf("epd7in5b: close: release port: %w", err))
		}
	}
	return errors.Join(errs...)
}

// validate checks the frame before anything reaches the panel.
func (d *Display) validate(f display.Frame) error {
	if f.Image == nil {
		return errors.New("epd7in5b: frame has no image")
	}
	bounds := d.surface.Bounds
	if got := f.Image.Bounds(); got != bounds {
		return fmt.Errorf("epd7in5b: frame bounds %v, the panel is %v", got, bounds)
	}
	switch f.Mode {
	case display.ModeFull, display.ModeFast, display.ModeWash:
		return nil
	case display.ModePartial:
		if f.Window.Intersect(bounds).Empty() {
			return fmt.Errorf("epd7in5b: partial window %v is empty or outside the panel %v", f.Window, bounds)
		}
		return nil
	default:
		return fmt.Errorf("epd7in5b: unknown refresh mode %d", int(f.Mode))
	}
}

// refresh powers the panel up for the frame's mode and transfers it. Once a
// power-up has been attempted the panel counts as awake.
func (d *Display) refresh(ctx context.Context, f display.Frame) error {
	d.awake = true
	switch f.Mode {
	case display.ModeFast:
		if err := d.panel.InitFast(ctx); err != nil {
			return err
		}
		return d.panel.Display(ctx, f.Image)
	case display.ModePartial:
		if err := d.panel.InitPartial(ctx); err != nil {
			return err
		}
		return d.panel.DisplayPartial(ctx, f.Image, f.Window)
	case display.ModeWash:
		if err := d.panel.Init(ctx); err != nil {
			return err
		}
		if err := d.panel.Clear(ctx); err != nil {
			return err
		}
		return d.panel.Display(ctx, f.Image)
	default: // ModeFull; validate has refused anything else
		if err := d.panel.Init(ctx); err != nil {
			return err
		}
		return d.panel.Display(ctx, f.Image)
	}
}

// sleep puts the panel to sleep on the display's own budget, detached from
// ctx's cancellation: a cancelled refresh still ends with the panel off.
// After a cancelled refresh the panel may still be busy and the attempt may
// fail; the panel then stays awake for the next Init or Close to settle.
func (d *Display) sleep(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sleepTimeout)
	defer cancel()
	if err := d.panel.Sleep(ctx); err != nil {
		return fmt.Errorf("epd7in5b: sleep: %w", err)
	}
	d.awake = false
	return nil
}
