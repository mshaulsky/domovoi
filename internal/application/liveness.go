package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// liveness checks that the application still makes progress: the core's
// mutex is responsive and every render coordinator kept its promise of a
// progress mark (Due). Its first failure stops the application with that
// failure as the cause, so any supervisor sees a non-zero exit.
type liveness struct {
	core         pinger
	coordinators []progressor
	log          *slog.Logger
}

// pinger is the core as the liveness check sees it.
type pinger interface {
	Ping(ctx context.Context) error
}

// progressor is a render coordinator as the liveness check sees it.
type progressor interface {
	Name() string
	Due() time.Time
}

const (
	// livenessEvery is the check cadence, well inside the shortest promise
	// of two one-minute ticks.
	livenessEvery = 30 * time.Second
	// pingTimeout bounds the core probe; the mutex is held for milliseconds.
	pingTimeout = 5 * time.Second
	// shutdownSlack is added to the shutdown budget before the hard exit, so
	// a stop that finishes right at its deadline is not cut short.
	shutdownSlack = 5 * time.Second
)

// exit ends the process; tests swap it for a recorder.
var exit = os.Exit

// check probes the core, then every coordinator's promise, and returns the
// first problem.
func (l *liveness) check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := l.core.Ping(ctx); err != nil {
		return fmt.Errorf("liveness: %w", err)
	}
	now := time.Now()
	for _, co := range l.coordinators {
		if due := co.Due(); !due.IsZero() && now.After(due) {
			return fmt.Errorf(
				"liveness: display %s overdue by %s: no progress since one was due at %s",
				co.Name(),
				now.Sub(due).Round(time.Second),
				due.Format(time.RFC3339),
			)
		}
	}
	return nil
}

// run checks every interval until ctx is done or a check fails; the first
// failure is logged and handed to fail, and run returns: the application
// is stopping.
func (l *liveness) run(ctx context.Context, every time.Duration, fail func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := l.check(ctx)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return // a probe cut short by the shutdown is no failure
			}
			l.log.Error("liveness check failed", "err", err)
			fail(err)
			return
		}
	}
}

// shutdownGuard exits the process with code 2 unless disarmed within d: a
// deadlocked core would hang the graceful stop forever. Under systemd the
// watchdog would kill it too; without systemd this is the only guarantee.
func shutdownGuard(d time.Duration, log *slog.Logger) (disarm func()) {
	t := time.AfterFunc(d, func() {
		log.Error("shutdown exceeded its budget", "budget", d)
		exit(2)
	})
	return func() { t.Stop() }
}
