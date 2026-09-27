package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"time"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/scene"
)

// Coordinator renders one display.
type Coordinator struct {
	cfg     Config
	disp    Display
	scenes  []Scene
	state   StateReader
	locale  Locale
	log     *slog.Logger
	metrics Metrics

	wake chan struct{}

	busy sync.Mutex // one render at a time

	mu       sync.Mutex // guards page, last, lastFull, gen
	page     int
	last     *image.Paletted // what is on the glass; nil = unknown
	lastFull time.Time
	gen      uint64 // bumped by Refresh, so a refresh during a render is not lost
}

// Config tunes a coordinator.
type Config struct {
	Name       string        // the display's configured name
	Tick       time.Duration // render cadence, aligned to the wall clock
	FullEvery  time.Duration // an unchanged frame is still redrawn this often
	StartGrace time.Duration // how long to wait for the first Wake before rendering anyway
}

// FixedLocale is a Locale that never changes: the configured language and
// time zone.
type FixedLocale struct {
	bundle   *i18n.Bundle
	location *time.Location
}

// DefaultStartGrace is how long a coordinator waits at start for the sources
// to deliver before it shows the starting frame.
const DefaultStartGrace = 30 * time.Second

// NewFixedLocale returns a Locale for one bundle and zone; a nil zone means
// UTC.
func NewFixedLocale(b *i18n.Bundle, loc *time.Location) FixedLocale {
	if loc == nil {
		loc = time.UTC
	}
	return FixedLocale{bundle: b, location: loc}
}

// New validates the config and prepares the coordinator; nothing is drawn
// until Run or Render.
func New(cfg Config, disp Display, scenes []Scene, st StateReader, locale Locale, log *slog.Logger, m Metrics) (*Coordinator, error) {
	if cfg.Name == "" {
		return nil, errors.New("render: display name is required")
	}
	if cfg.Tick <= 0 {
		return nil, fmt.Errorf("render: %s: tick must be positive", cfg.Name)
	}
	if len(scenes) == 0 {
		return nil, fmt.Errorf("render: %s: at least one scene is required", cfg.Name)
	}
	if locale == nil {
		return nil, fmt.Errorf("render: %s: locale is required", cfg.Name)
	}
	if locale.Bundle() == nil {
		return nil, fmt.Errorf("render: %s: language bundle is required", cfg.Name)
	}
	if cfg.StartGrace <= 0 {
		cfg.StartGrace = DefaultStartGrace
	}
	return &Coordinator{
		cfg:     cfg,
		disp:    disp,
		scenes:  scenes,
		state:   st,
		locale:  locale,
		log:     log.With("component", "render", "display", cfg.Name),
		metrics: m,
		wake:    make(chan struct{}, 1),
	}, nil
}

// Bundle returns the fixed bundle.
func (l FixedLocale) Bundle() *i18n.Bundle {
	return l.bundle
}

// Location returns the fixed zone.
func (l FixedLocale) Location() *time.Location {
	return l.location
}

// Name returns the display's configured name.
func (c *Coordinator) Name() string {
	return c.cfg.Name
}

// Run renders until ctx is done: first after the start grace or the first
// Wake, whichever comes first, then on every tick and every Wake. Render
// errors are logged, not returned; stopping is not a failure.
func (c *Coordinator) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case <-c.wake:
	case <-time.After(c.cfg.StartGrace):
	}
	for {
		if err := c.Render(ctx); err != nil && ctx.Err() == nil {
			c.log.Warn("render failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-c.wake:
		case <-time.After(c.untilNextTick()):
		}
	}
}

// Render draws the current page once and shows it if it changed or a full
// refresh is due. The state lock is held only around the bookkeeping, never
// across Show, which may take a panel's full refresh time — commands must
// not wait for it.
func (c *Coordinator) Render(ctx context.Context) error {
	c.busy.Lock()
	defer c.busy.Unlock()
	now := time.Now()

	c.mu.Lock()
	sc := c.scenes[c.page%len(c.scenes)]
	last, lastFull, gen := c.last, c.lastFull, c.gen
	c.mu.Unlock()

	surface := c.disp.Surface()
	img, err := sc.Render(surface, c.view(now))
	if err != nil {
		return fmt.Errorf("render %s/%s: %w", c.cfg.Name, sc.Name(), err)
	}
	if last != nil && bytes.Equal(img.Pix, last.Pix) && now.Sub(lastFull) < c.cfg.FullEvery {
		c.log.Debug("frame unchanged, skipped")
		return nil
	}
	frame := display.Frame{Image: img, Mode: display.ModeFull}
	if err := c.disp.Show(ctx, frame); err != nil {
		c.forget()            // the glass is unknown either way
		if ctx.Err() == nil { // a Show cut short by shutdown is not a display failure
			c.metrics.IncRenderError(c.cfg.Name)
		}
		return fmt.Errorf("show %s: %w", c.cfg.Name, err)
	}
	c.mu.Lock()
	if c.gen == gen { // no Refresh arrived meanwhile
		c.last, c.lastFull = img, now
	}
	c.mu.Unlock()
	took := time.Since(now)
	c.metrics.IncRender(c.cfg.Name, frame.Mode.String())
	c.metrics.ObserveRenderDuration(c.cfg.Name, took)
	c.log.Info("frame shown", "scene", sc.Name(), "mode", frame.Mode, "took", took)
	return nil
}

// Wake asks for a render outside the tick: an alert changed, a command
// arrived. Wakes coalesce.
func (c *Coordinator) Wake() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// NextPage advances to the next scene and wakes.
func (c *Coordinator) NextPage() {
	c.mu.Lock()
	c.page = (c.page + 1) % len(c.scenes)
	c.mu.Unlock()
	c.Wake()
}

// Refresh forgets what is on the glass, so the next render is a full one,
// and wakes.
func (c *Coordinator) Refresh() {
	c.forget()
	c.Wake()
}

// forget marks the glass as unknown.
func (c *Coordinator) forget() {
	c.mu.Lock()
	c.last = nil
	c.gen++
	c.mu.Unlock()
}

// view assembles the scene's input from a snapshot.
func (c *Coordinator) view(now time.Time) scene.View {
	sn := c.state.Snapshot(now)
	v := scene.View{
		Now:      now,
		Location: c.locale.Location(),
		Bundle:   c.locale.Bundle(),
		Devices:  sn.Devices,
		Readings: sn.Readings,
		Stale:    make(map[model.DeviceID]bool, len(sn.Devices)),
		Seen:     make(map[model.DeviceID]time.Time, len(sn.Devices)),
	}
	for _, d := range sn.Devices {
		v.Stale[d.ID] = sn.Stale(d.ID)
		if seen, ok := sn.Seen(d.ID); ok {
			v.Seen[d.ID] = seen
		}
	}
	// Starting: there are sources and none has answered yet, with data or
	// with an error. A dead cloud must not keep the starting frame up.
	v.Starting = len(sn.Health) > 0
	for _, name := range sn.Sources() {
		h := sn.Health[name]
		if !h.LastOK.IsZero() || !h.LastError.IsZero() {
			v.Starting = false
		}
		v.Sources = append(v.Sources, scene.SourceStatus{Name: name, LastOK: h.LastOK, Stale: sn.SourceStale(name)})
	}
	return v
}

// untilNextTick returns the wait to the next tick boundary on the wall
// clock, so several displays with the same tick render together.
func (c *Coordinator) untilNextTick() time.Duration {
	now := time.Now()
	next := now.Truncate(c.cfg.Tick).Add(c.cfg.Tick)
	return next.Sub(now)
}
