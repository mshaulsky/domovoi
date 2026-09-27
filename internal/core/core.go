package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/state"
)

// Core owns the state and runs the loops.
type Core struct {
	state   *state.State
	log     *slog.Logger
	metrics Metrics

	mu           sync.Mutex
	pollers      map[string]Poller
	coordinators map[string]Coordinator
	pending      map[string]bool // sources that have not answered at all since start
	awaiting     map[string]bool // sources that have not delivered data since start
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	started      bool
}

// ErrNotImplemented marks commands whose feature is a later stage.
var ErrNotImplemented = errors.New("not implemented yet")

// New returns a core around a state.
func New(st *state.State, log *slog.Logger, m Metrics) *Core {
	return &Core{
		state:        st,
		log:          log.With("component", "core"),
		metrics:      m,
		pollers:      map[string]Poller{},
		coordinators: map[string]Coordinator{},
		pending:      map[string]bool{},
		awaiting:     map[string]bool{},
	}
}

// AddPoller registers a source's poller under the source name.
func (c *Core) AddPoller(name string, p Poller) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pollers[name] = p
	c.pending[name] = true
	c.awaiting[name] = true
}

// AddCoordinator registers a display's coordinator.
func (c *Core) AddCoordinator(co Coordinator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.coordinators[co.Name()] = co
}

// Batch applies a source's pass atomically: devices and readings into the
// state and the source marked healthy, then the changes into events.
func (c *Core) Batch(name string, b source.Batch) {
	changes := c.state.Apply(name, b.Devices, b.Readings, time.Now())
	for _, e := range state.Classify(changes) {
		c.journal(e)
	}
	c.log.Debug("batch applied", "source", name, "devices", len(b.Devices), "readings", len(b.Readings), "changes", len(changes))
	c.settle(name, true)
}

// Event records an event a watcher produced (a button press).
func (c *Core) Event(e model.Event) {
	c.journal(e)
}

// Health records a source's failure or recovery. A failing source still
// counts as reported: startup must not wait for a dead cloud.
func (c *Core) Health(name string, err error) {
	c.state.SetHealth(name, err, time.Now())
	c.settle(name, false)
}

// Start runs every poller and coordinator in the background.
func (c *Core) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return errors.New("core: already started")
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.started = true
	for name, p := range c.pollers {
		c.wg.Go(func() {
			if err := p.Run(ctx); err != nil {
				c.log.Error("poller stopped", "source", name, "err", err)
			}
		})
	}
	for name, co := range c.coordinators {
		c.wg.Go(func() {
			if err := co.Run(ctx); err != nil {
				c.log.Error("coordinator stopped", "display", name, "err", err)
			}
		})
	}
	c.log.Info("started", "sources", len(c.pollers), "displays", len(c.coordinators))
	return nil
}

// Stop cancels the loops and waits for them, or for ctx's deadline.
func (c *Core) Stop(ctx context.Context) error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}
	c.cancel()
	c.started = false
	c.mu.Unlock()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		c.log.Info("stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("core: stop: %w", ctx.Err())
	}
}

// Once polls every source a single time, then renders every display once.
// Every source is tried even if one fails; the errors are joined.
func (c *Core) Once(ctx context.Context) error {
	c.mu.Lock()
	pollers := maps.Clone(c.pollers)
	coordinators := slices.Collect(maps.Values(c.coordinators))
	c.mu.Unlock()
	var errs []error
	for name, p := range pollers {
		if err := p.Once(ctx); err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", name, err))
		}
	}
	for _, co := range coordinators {
		if err := co.Render(ctx); err != nil {
			errs = append(errs, fmt.Errorf("display %s: %w", co.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// NextPage advances a display to its next scene.
func (c *Core) NextPage(display string) error {
	co, err := c.coordinator(display)
	if err != nil {
		return err
	}
	co.NextPage()
	return nil
}

// Refresh forces a full refresh of a display.
func (c *Core) Refresh(display string) error {
	co, err := c.coordinator(display)
	if err != nil {
		return err
	}
	co.Refresh()
	return nil
}

// Acknowledge marks one alert as seen. Alerts arrive in a later stage.
func (c *Core) Acknowledge(int64) error {
	return fmt.Errorf("core: acknowledge: %w", ErrNotImplemented)
}

// AcknowledgeAll marks every alert as seen. Alerts arrive in a later stage.
func (c *Core) AcknowledgeAll() error {
	return fmt.Errorf("core: acknowledge all: %w", ErrNotImplemented)
}

// Mute silences notifications. Notifications arrive in a later stage.
func (c *Core) Mute(time.Duration) error {
	return fmt.Errorf("core: mute: %w", ErrNotImplemented)
}

// Reload re-reads the configuration. Reload arrives in a later stage.
func (c *Core) Reload() error {
	return fmt.Errorf("core: reload: %w", ErrNotImplemented)
}

// journal records an event: logged and counted now, stored from stage 2.
func (c *Core) journal(e model.Event) {
	c.metrics.IncEvent(string(e.Kind))
	c.log.Info("event", "kind", e.Kind, "device", e.Device, "detail", e.Detail, "at", e.At)
}

// settle notes a source's answer and wakes the displays when the picture
// is worth a frame: when the last configured source has answered at all
// (with data or with an error — startup must not wait for a dead cloud),
// and later when a source that started with an error delivers its first
// data, so the real picture does not wait for the next tick. Nothing wakes
// while a source is still pending: its answer will.
func (c *Core) settle(name string, delivered bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	wasPending, wasAwaiting := c.pending[name], c.awaiting[name]
	delete(c.pending, name)
	if delivered {
		delete(c.awaiting, name)
	}
	if len(c.pending) > 0 {
		return
	}
	worthAFrame := wasPending || (delivered && wasAwaiting)
	if !worthAFrame {
		return
	}
	c.log.Info("waking displays", "source", name, "still_without_data", len(c.awaiting))
	for _, co := range c.coordinators {
		co.Wake()
	}
}

func (c *Core) coordinator(display string) (Coordinator, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	co, ok := c.coordinators[display]
	if !ok {
		return nil, fmt.Errorf("core: unknown display %q", display)
	}
	return co, nil
}
