package core

import (
	"context"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=core

// Metrics counts what the core sees.
type Metrics interface {
	IncEvent(kind string)
	IncStoreError(op string)
}

// Store is where the core persists and restores; nil means no storage
// (tests, a deliberately stateless run). One Persist is one transaction.
type Store interface {
	Persist(ctx context.Context, devices []model.Device, readings []model.Reading, events []model.Event, at time.Time) error
	Restore(ctx context.Context) ([]model.Device, []model.Reading, error)
	Prune(ctx context.Context, before time.Time) (int64, error)
}

// Poller polls one source until stopped; Once polls it a single time.
type Poller interface {
	Run(ctx context.Context) error
	Once(ctx context.Context) error
}

// Coordinator renders one display until stopped; Render draws once.
type Coordinator interface {
	Name() string
	Run(ctx context.Context) error
	Render(ctx context.Context) error
	Wake()
	NextPage()
	Refresh()
}

// Commands is what inputs may ask of the core: the button, the back office
// and Telegram call this and nothing else.
type Commands interface {
	NextPage(display string) error
	Refresh(display string) error
	Acknowledge(alertID int64) error
	AcknowledgeAll() error
	Mute(d time.Duration) error
	Reload() error
}
