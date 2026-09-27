package container

import (
	"log/slog"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/core"
	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/metrics"
	"github.com/mshaulsky/domovoi/internal/scene"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/state"
)

// Container is the manifest of singletons and the registries of kinds.
// Sources, displays and scenes are instances, not singletons: they go
// through the registries and are built from the configuration.
type Container struct {
	Config  Lazy[*config.Config]
	Logger  Lazy[*slog.Logger]
	Metrics Lazy[*metrics.Registry]
	State   Lazy[*state.State]
	Core    Lazy[*core.Core]

	Sources  Registry[SourceConstructor]
	Displays Registry[DisplayConstructor]
	Scenes   Registry[scene.Scene]

	fail func(error)
}

// SourceConstructor builds a source instance from its configuration
// section; the glue that registers it decodes the kind-specific options.
type SourceConstructor func(cfg config.SourceSection) (source.Source, error)

// DisplayConstructor builds a display instance from its configuration
// section.
type DisplayConstructor func(cfg config.DisplaySection) (display.Display, error)

// New returns a container whose Fail reports a fatal failure to the
// application, which stops everything with that cause.
func New(fail func(error)) *Container {
	if fail == nil {
		fail = func(error) {}
	}
	return &Container{fail: fail}
}

// Fail reports a fatal failure that happened after Start: a module that
// cannot go on. The application cancels the root context with the cause.
func (c *Container) Fail(err error) {
	c.fail(err)
}
