package render

import (
	"context"
	"image"
	"time"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/scene"
	"github.com/mshaulsky/domovoi/internal/state"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=render

// Display is the panel or file the coordinator draws on.
type Display interface {
	Surface() display.Surface
	Show(ctx context.Context, f display.Frame) error
}

// Scene renders a page. It restates scene.Scene so this package's mocks
// come from its own interfaces.go; the assertion below keeps the two equal.
type Scene interface {
	Name() string
	Render(s display.Surface, v scene.View) (*image.Paletted, error)
}

var _ Scene = scene.Scene(nil)

// History answers what the store remembers, for the tiles: today's
// extremes, the trend over the last hour, the latest journal events. A
// nil History means no storage: the tiles draw without them.
type History interface {
	Extremes(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (lo, hi float64, ok bool, err error)
	Trend(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (delta float64, ok bool, err error)
	Events(ctx context.Context, since time.Time, limit int) ([]model.Event, error)
}

// Locale is where the coordinator asks, before every frame, which language
// and time zone to draw in. A settings layer may change the answer at
// runtime without rebuilding the coordinator.
type Locale interface {
	Bundle() *i18n.Bundle
	Location() *time.Location
}

// StateReader hands out snapshots.
type StateReader interface {
	Snapshot(now time.Time) state.Snapshot
}

// Metrics counts what the coordinator does.
type Metrics interface {
	IncRender(display, mode string)
	ObserveRenderDuration(display string, d time.Duration)
	IncRenderError(display string)
}
