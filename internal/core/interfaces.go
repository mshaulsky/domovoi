package core

import (
	"context"
	"time"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=core

// Metrics counts what the core sees.
type Metrics interface {
	IncEvent(kind string)
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
