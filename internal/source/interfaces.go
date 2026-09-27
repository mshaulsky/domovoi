package source

import (
	"context"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=source

// Source is a polled provider: one Poll is one pass. Batch.Devices carries
// the device list whenever the source has it — on the first pass always,
// later whenever it changed or whenever the vendor call returns it anyway;
// nil means "unchanged since the previous batch".
type Source interface {
	Name() string
	Poll(ctx context.Context) (Batch, error)
}

// Watcher is a push provider (MQTT). It delivers into the same Sink the
// Poller uses, so the core has exactly one intake.
type Watcher interface {
	Watch(ctx context.Context, sink Sink) error
}

// Sink is the core's intake. A source failure is reported through Health,
// never as fabricated readings: the lock did not go offline, the cloud did.
// A Batch marks its source healthy by itself; Health with a nil error is
// for a recovery that brings no data (a watcher reconnecting).
type Sink interface {
	Batch(source string, b Batch)
	Event(e model.Event)
	Health(source string, err error)
}

// Controller is the optional write side, for the automation hook: sources
// that can act implement it; the core refuses actions on those that do not.
type Controller interface {
	Set(ctx context.Context, device model.DeviceID, metric model.Metric, v model.Value) error
}

// Metrics counts what the Poller does.
type Metrics interface {
	IncPollSuccess(source string)
	IncPollError(source string)
	ObservePollDuration(source string, d time.Duration)
}
