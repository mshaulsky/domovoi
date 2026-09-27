package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/core"
	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/metrics"
	"github.com/mshaulsky/domovoi/internal/render"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/state"
)

// Options is what the command line decides.
type Options struct {
	ConfigPath string
	Check      bool               // load, resolve, build everything, touch nothing outside the process, exit
	Once       bool               // poll every source once, render every display once, exit
	Version    string             // for the log line
	Lookup     config.Lookup      // secret resolution; nil = config.DefaultLookup
	Modules    []container.Module // nil = the compiled-in list
	LogOutput  io.Writer          // nil = stderr
	LogLevel   slog.Level
}

// StopTimeout bounds the shutdown: the panel may be mid-frame.
const StopTimeout = 20 * time.Second

// Run is the whole program after flag parsing. It returns when ctx is
// done, when a module fails fatally, or after -check / -once completed.
func Run(ctx context.Context, opts Options) error {
	log := newLogger(opts)
	log.Info("domovoi", "version", opts.Version, "config", opts.ConfigPath)

	lookup := opts.Lookup
	if lookup == nil {
		lookup = config.DefaultLookup()
	}
	cfg, err := config.Load(opts.ConfigPath, lookup)
	if err != nil {
		return err
	}

	parent := ctx
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	c := container.New(func(err error) { cancel(err) })
	c.Check = opts.Check
	c.Config.Set(&cfg)
	c.Logger.Set(log)
	c.Metrics.Set(metrics.New())

	mods := opts.Modules
	if mods == nil {
		mods = enabled()
	}
	for _, m := range mods {
		if err := m.Register(c); err != nil {
			return fmt.Errorf("register module %s: %w", m.Name(), err)
		}
	}
	displays, err := assemble(c, cfg, mods)
	defer closeDisplays(context.WithoutCancel(ctx), displays, log) // also the ones built before a failure
	if err != nil {
		return err
	}

	switch {
	case opts.Check:
		log.Info("configuration OK", "sources", len(cfg.Sources), "displays", len(cfg.Displays),
			"source_kinds", c.Sources.Kinds(), "display_kinds", c.Displays.Kinds(), "scenes", c.Scenes.Kinds())
		stopAll(mods, log, StopTimeout) // nothing started, but Register opened things
		return nil
	case opts.Once:
		co, err := c.Core.Get()
		if err != nil {
			return err
		}
		err = co.Once(ctx)
		stopAll(mods, log, StopTimeout)
		return err
	}

	stopTimeout := stopTimeoutFor(displays)
	started, err := startAll(ctx, mods, log)
	if err != nil {
		stopAll(started, log, stopTimeout)
		return err
	}
	sd := notifier{log: log}
	sd.ready()
	watchdogCtx, stopWatchdog := context.WithCancel(context.WithoutCancel(ctx)) // fed until the modules have stopped
	var wg sync.WaitGroup
	wg.Go(func() { sd.watchdog(watchdogCtx) })
	<-ctx.Done()
	sd.stopping()
	stopAll(started, log, stopTimeout)
	stopWatchdog()
	wg.Wait()
	if parent.Err() != nil {
		return nil // asked to stop: not a failure
	}
	return context.Cause(ctx)
}

// startSlack is added to the longest poll timeout to get the start grace:
// the first frame should show the first answers, not precede them.
const startSlack = 5 * time.Second

// assemble builds the configured instances — a source and its poller per
// source section, a display and its coordinator per display section — then
// lets the modules that implement container.Assembler add their wiring.
// Displays built before a failure are returned so the caller can close them.
func assemble(c *container.Container, cfg config.Config, mods []container.Module) ([]display.Display, error) {
	w, err := newWiring(c, cfg)
	if err != nil {
		return nil, err
	}
	grace, err := w.sources()
	if err != nil {
		return nil, err
	}
	displays, err := w.displays(grace)
	if err != nil {
		return displays, err
	}
	for _, m := range mods {
		if a, ok := m.(container.Assembler); ok {
			if err := a.Assemble(c, cfg); err != nil {
				return displays, fmt.Errorf("assemble module %s: %w", m.Name(), err)
			}
		}
	}
	return displays, nil
}

// wiring is what every instance is built with.
type wiring struct {
	c       *container.Container
	cfg     config.Config
	log     *slog.Logger
	metrics *metrics.Registry
	state   *state.State
	core    *core.Core
	history render.History // nil without a storage module
}

func newWiring(c *container.Container, cfg config.Config) (wiring, error) {
	log, err := c.Logger.Get()
	if err != nil {
		return wiring{}, err
	}
	m, err := c.Metrics.Get()
	if err != nil {
		return wiring{}, err
	}
	st, err := c.State.Get()
	if err != nil {
		return wiring{}, err
	}
	co, err := c.Core.Get()
	if err != nil {
		return wiring{}, err
	}
	w := wiring{c: c, cfg: cfg, log: log, metrics: m, state: st, core: co}
	switch s, err := c.Store.Get(); {
	case err == nil:
		w.history = s
	case errors.Is(err, container.ErrNotProvided):
	default:
		return wiring{}, err
	}
	return w, nil
}

// sources builds a source and its poller per section. It returns the start
// grace the coordinators should allow: at least one poll timeout, so a slow
// cloud does not cost a "starting" frame that the first answer replaces a
// moment later.
func (w wiring) sources() (time.Duration, error) {
	grace := render.DefaultStartGrace
	for _, sc := range w.cfg.Sources {
		ctor, err := w.c.Sources.Get(sc.Kind)
		if err != nil {
			return 0, fmt.Errorf("source %s: %w", sc.Name, err)
		}
		src, err := ctor(sc)
		if err != nil {
			return 0, err
		}
		w.state.SetStaleAfter(sc.Name, sc.StaleAfter)
		poller := source.NewPoller(src, w.core, source.PollerConfig{Interval: sc.Interval}, w.log, w.metrics)
		w.core.AddPoller(sc.Name, poller)
		grace = max(grace, sc.Interval+startSlack)
	}
	return grace, nil
}

// displays builds a display and its coordinator per section.
func (w wiring) displays(grace time.Duration) ([]display.Display, error) {
	var displays []display.Display
	for _, dc := range w.cfg.Displays {
		ctor, err := w.c.Displays.Get(dc.Kind)
		if err != nil {
			return displays, fmt.Errorf("display %s: %w", dc.Name, err)
		}
		disp, err := ctor(dc)
		if err != nil {
			return displays, err
		}
		displays = append(displays, disp)
		scenes := make([]render.Scene, 0, len(dc.Scenes))
		for _, name := range dc.Scenes {
			sc, err := w.c.Scenes.Get(name)
			if err != nil {
				return displays, fmt.Errorf("display %s: scene %s: %w", dc.Name, name, err)
			}
			scenes = append(scenes, sc)
		}
		bundle, err := i18n.Load(dc.Language)
		if err != nil {
			return displays, fmt.Errorf("display %s: %w", dc.Name, err)
		}
		cfg := render.Config{Name: dc.Name, Tick: dc.Tick, FullEvery: dc.FullEvery, StartGrace: grace}
		co, err := render.New(cfg, disp, scenes, w.state, render.NewFixedLocale(bundle, w.cfg.Timezone), w.history, w.log, w.metrics)
		if err != nil {
			return displays, err
		}
		w.core.AddCoordinator(co)
	}
	return displays, nil
}

// stopTimeoutFor is StopTimeout plus the slowest panel's full refresh, so a
// frame in flight can finish before the display is put to sleep.
func stopTimeoutFor(displays []display.Display) time.Duration {
	d := StopTimeout
	for _, disp := range displays {
		d = max(d, StopTimeout+disp.Surface().FullRefresh)
	}
	return d
}

// startAll starts modules top-down and returns the ones that started.
func startAll(ctx context.Context, mods []container.Module, log *slog.Logger) ([]container.Module, error) {
	var started []container.Module
	for _, m := range mods {
		if err := m.Start(ctx); err != nil {
			return started, fmt.Errorf("start module %s: %w", m.Name(), err)
		}
		started = append(started, m)
		log.Debug("module started", "module", m.Name())
	}
	return started, nil
}

// stopAll stops modules bottom-up within timeout, logging failures. It is
// also called for modules that never started: a module releases in Stop
// what it opened in Register, the database above all.
func stopAll(started []container.Module, log *slog.Logger, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, m := range slices.Backward(started) {
		if err := m.Stop(ctx); err != nil {
			log.Error("module stop failed", "module", m.Name(), "err", err)
		}
	}
}

// closeDisplays releases the displays the application created — the
// panel goes to sleep — after everything that drew on them has stopped.
func closeDisplays(ctx context.Context, displays []display.Display, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, StopTimeout)
	defer cancel()
	for _, d := range displays {
		if err := d.Close(ctx); err != nil {
			log.Error("display close failed", "err", err)
		}
	}
}

func newLogger(opts Options) *slog.Logger {
	out := opts.LogOutput
	if out == nil {
		out = os.Stderr
	}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: opts.LogLevel}))
}
