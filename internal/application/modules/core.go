package modules

import (
	"context"
	"fmt"

	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/core"
	"github.com/mshaulsky/domovoi/internal/state"
)

// Core provides the State and Core singletons and runs the core's loops.
// It is a pointer in the module list because Stop must reach the core it
// started.
type Core struct {
	core *core.Core
}

// Name returns the module name.
func (*Core) Name() string {
	return "core"
}

// Register provides the singletons.
func (m *Core) Register(c *container.Container) error {
	c.State.Provide(func() (*state.State, error) { return state.New(), nil })
	c.Core.Provide(func() (*core.Core, error) {
		st, err := c.State.Get()
		if err != nil {
			return nil, err
		}
		log, err := c.Logger.Get()
		if err != nil {
			return nil, err
		}
		metrics, err := c.Metrics.Get()
		if err != nil {
			return nil, err
		}
		m.core = core.New(st, log, metrics)
		return m.core, nil
	})
	return nil
}

// Start runs the pollers and coordinators.
func (m *Core) Start(ctx context.Context) error {
	if m.core == nil {
		return fmt.Errorf("core module: Start before the core was built")
	}
	return m.core.Start(ctx)
}

// Stop stops them.
func (m *Core) Stop(ctx context.Context) error {
	if m.core == nil {
		return nil
	}
	return m.core.Stop(ctx)
}
