package container

import (
	"context"

	"github.com/mshaulsky/domovoi/internal/config"
)

// Module is one compiled-in capability. Register adds what it provides to
// the container (kinds, singletons, routes) and starts nothing; Start
// spawns its background work and returns; Stop waits for it, within the
// context's deadline. Whoever creates, closes: a module releases in Stop
// what it built in Register or Start.
type Module interface {
	Name() string
	Register(c *Container) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Assembler is implemented by a module that wires instances once every
// module has registered: it sees the complete registries and the whole
// configuration. The application calls it after its own wiring of sources
// and displays and before Start, so a module that must hook into the core
// (alerts, storage restore, a Telegram bot) adds its wiring here instead of
// growing the application's.
type Assembler interface {
	Assemble(c *Container, cfg config.Config) error
}

// Passive is embedded by modules with no background work.
type Passive struct{}

// Start does nothing.
func (Passive) Start(context.Context) error {
	return nil
}

// Stop does nothing.
func (Passive) Stop(context.Context) error {
	return nil
}
