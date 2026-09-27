package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

// notifier reports the lifecycle to systemd and keeps its watchdog fed.
// Without NOTIFY_SOCKET (no systemd, -once, tests) every call is a no-op.
// Until the liveness check of stage 3 exists, the ping asserts only that the
// application goroutine runs.
type notifier struct {
	log *slog.Logger
}

// ready tells systemd the service is up (Type=notify).
func (n notifier) ready() {
	n.send(daemon.SdNotifyReady)
}

// stopping tells systemd a shutdown began.
func (n notifier) stopping() {
	n.send(daemon.SdNotifyStopping)
}

// watchdog pings systemd at half the WatchdogSec interval until ctx is done.
func (n notifier) watchdog(ctx context.Context) {
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		n.log.Warn("systemd watchdog settings unreadable", "err", err)
		return
	}
	if interval == 0 {
		return
	}
	t := time.NewTicker(interval / 2)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.send(daemon.SdNotifyWatchdog)
		}
	}
}

func (n notifier) send(state string) {
	if _, err := daemon.SdNotify(false, state); err != nil {
		n.log.Warn("systemd notify failed", "state", state, "err", err)
	}
}
