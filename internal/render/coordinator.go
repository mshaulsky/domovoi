package render

import (
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
	history History // nil: no storage
	log     *slog.Logger
	metrics Metrics

	wake chan struct{}

	busy   sync.Mutex // one render at a time; also guards policy
	policy *Policy

	mu   sync.Mutex // guards page, last, gen, mark; never held across Show
	page int
	last *image.Paletted // what is on the glass; nil = unknown
	gen  uint64          // bumped by Refresh, so a refresh during a render is not lost
	mark time.Time       // the last progress mark; before the first render, the end of the start grace
}

// Config tunes a coordinator. The cadence and the refresh decisions are
// the policy's, tuned by PolicyConfig.
type Config struct {
	Name       string                    // the display's configured name
	StartGrace time.Duration             // how long to wait for the first Wake before rendering anyway
	Labels     map[model.DeviceID]string // short device names for the hallway strip
}

// FixedLocale is a Locale that never changes: the configured language and
// time zone.
type FixedLocale struct {
	bundle   *i18n.Bundle
	location *time.Location
}

// Timing constants.
const (
	// DefaultStartGrace is how long a coordinator waits at start for the
	// sources to deliver before it shows the starting frame.
	DefaultStartGrace = 30 * time.Second
	// trendWindow is how far back a tile's trend arrow looks.
	trendWindow = time.Hour
	// eventsWindow is how far back the footer's events reach.
	eventsWindow = 24 * time.Hour
	// eventsLimit is how many events a view carries.
	eventsLimit = 5
	// historyTimeout bounds the history queries of one frame.
	historyTimeout = 5 * time.Second
)

// NewFixedLocale returns a Locale for one bundle and zone; a nil zone means
// UTC.
func NewFixedLocale(b *i18n.Bundle, loc *time.Location) FixedLocale {
	if loc == nil {
		loc = time.UTC
	}
	return FixedLocale{bundle: b, location: loc}
}

// New validates the config and the collaborators, the refresh policy among
// them, and prepares the coordinator; nothing is drawn until Run or Render.
func New(
	cfg Config,
	policy *Policy,
	disp Display,
	scenes []Scene,
	st StateReader,
	locale Locale,
	history History,
	log *slog.Logger,
	m Metrics,
) (*Coordinator, error) {
	if cfg.Name == "" {
		return nil, errors.New("render: display name is required")
	}
	if policy == nil {
		return nil, fmt.Errorf("render: %s: policy is required", cfg.Name)
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
		history: history,
		log:     log.With("component", "render", "display", cfg.Name),
		metrics: m,
		wake:    make(chan struct{}, 1),
		policy:  policy,
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
	c.mu.Lock()
	c.mark = time.Now().Add(c.cfg.StartGrace) // the first frame is promised for the end of the grace
	c.mu.Unlock()
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

// Render draws the current page once and lets the policy decide whether
// and how it reaches the glass. The state lock is held only around the
// bookkeeping, never across Show, which may take a panel's full refresh
// time — commands must not wait for it.
func (c *Coordinator) Render(ctx context.Context) error {
	c.busy.Lock()
	defer c.busy.Unlock()
	now := time.Now()

	c.mu.Lock()
	c.mark = now
	sc := c.scenes[c.page%len(c.scenes)]
	last, gen := c.last, c.gen
	c.mu.Unlock()

	surface := c.disp.Surface()
	img, err := sc.Render(surface, c.view(ctx, now))
	if err != nil {
		return fmt.Errorf("render %s/%s: %w", c.cfg.Name, sc.Name(), err)
	}
	// The policy reads the wall clock of the display's zone. Nothing
	// produces urgent renders before alerts (stage 4).
	local := now.In(c.locale.Location())
	d := c.policy.Decide(last, img, surface, local, false)
	if d.Skip {
		c.log.Debug("frame skipped", "scene", sc.Name(), "reason", d.Reason)
		return nil
	}
	frame := display.Frame{Image: img, Windows: d.Windows, Mode: d.Mode}
	err = c.disp.Show(ctx, frame)
	c.stamp() // a hung Show shows up only once it outlasts its own timeout
	if err != nil {
		c.forget() // the glass is unknown either way
		c.policy.Failed()
		if ctx.Err() == nil { // a Show cut short by shutdown is not a display failure
			c.metrics.IncRenderError(c.cfg.Name)
		}
		return fmt.Errorf("show %s: %w", c.cfg.Name, err)
	}
	c.policy.Shown(d, local)
	c.mu.Lock()
	if c.gen == gen { // no Refresh arrived meanwhile
		c.last = img
	}
	c.mu.Unlock()
	took := time.Since(now)
	c.metrics.IncRender(c.cfg.Name, frame.Mode.String())
	c.metrics.ObserveRenderDuration(c.cfg.Name, took)
	attrs := []any{"scene", sc.Name(), "mode", frame.Mode, "reason", d.Reason, "took", took}
	if frame.Mode == display.ModePartial {
		attrs = append(attrs, "windows", frame.Windows)
	}
	c.log.Info("frame shown", attrs...)
	return nil
}

// Due returns the moment by which the coordinator promises its next
// progress mark: two ticks after the last one, the tick being the policy's
// at that mark, so the quiet tick counts at night. Before the first render
// the mark is the end of the start grace. A Render stamps progress when it
// begins and when Show returns, so Due never waits behind a Show in
// flight. The zero time means nothing is promised: Run has not begun.
func (c *Coordinator) Due() time.Time {
	c.mu.Lock()
	mark := c.mark
	c.mu.Unlock()
	if mark.IsZero() {
		return time.Time{}
	}
	return mark.Add(2 * c.tickAt(mark))
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

// stamp marks progress now.
func (c *Coordinator) stamp() {
	c.mu.Lock()
	c.mark = time.Now()
	c.mu.Unlock()
}

// forget marks the glass as unknown.
func (c *Coordinator) forget() {
	c.mu.Lock()
	c.last = nil
	c.gen++
	c.mu.Unlock()
}

// view assembles the scene's input from a snapshot and, when there is a
// store, from history.
func (c *Coordinator) view(ctx context.Context, now time.Time) scene.View {
	sn := c.state.Snapshot(now)
	v := scene.View{
		Now:      now,
		Location: c.locale.Location(),
		Bundle:   c.locale.Bundle(),
		Devices:  sn.Devices,
		Readings: sn.Readings,
		Stale:    make(map[model.DeviceID]bool, len(sn.Devices)),
		Seen:     make(map[model.DeviceID]time.Time, len(sn.Devices)),
		Restored: sn.Restored,
		Labels:   c.cfg.Labels,
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
	c.enrich(ctx, &v)
	return v
}

// enrich adds what history knows: today's temperature extremes and the
// hour's trend per device, the latest events. A failing query costs one
// warning and that item, never the frame.
func (c *Coordinator) enrich(ctx context.Context, v *scene.View) {
	if c.history == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, historyTimeout)
	defer cancel()
	local := v.Now.In(v.Location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, v.Location)
	v.Extremes = map[model.DeviceID]map[model.Metric]scene.Extremes{}
	v.Trends = map[model.DeviceID]map[model.Metric]float64{}
	for _, d := range v.Devices {
		if !v.Has(d.ID, model.Temperature) {
			continue
		}
		if lo, hi, ok, err := c.history.Extremes(ctx, d.ID, model.Temperature, midnight); err != nil {
			c.log.Warn("extremes query failed", "device", d.ID, "err", err)
		} else if ok {
			v.Extremes[d.ID] = map[model.Metric]scene.Extremes{model.Temperature: {Min: lo, Max: hi}}
		}
		if delta, ok, err := c.history.Trend(ctx, d.ID, model.Temperature, v.Now.Add(-trendWindow)); err != nil {
			c.log.Warn("trend query failed", "device", d.ID, "err", err)
		} else if ok {
			v.Trends[d.ID] = map[model.Metric]float64{model.Temperature: delta}
		}
	}
	events, err := c.history.Events(ctx, v.Now.Add(-eventsWindow), eventsLimit)
	if err != nil {
		c.log.Warn("events query failed", "err", err)
		return
	}
	v.Events = events
}

// untilNextTick returns the wait to the next tick boundary on the wall
// clock, so several displays with the same tick render together. The
// policy names the tick: longer in quiet hours.
func (c *Coordinator) untilNextTick() time.Duration {
	now := time.Now()
	tick := c.tickAt(now)
	next := now.Truncate(tick).Add(tick)
	return next.Sub(now)
}

// tickAt is the policy's tick at t on the display's wall clock. Tick reads
// only the policy's configuration, so it needs no busy lock.
func (c *Coordinator) tickAt(t time.Time) time.Duration {
	return c.policy.Tick(t.In(c.locale.Location()))
}
