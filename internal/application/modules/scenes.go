package modules

import (
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/scene"
)

// Scenes provides every scene by name.
type Scenes struct {
	container.Passive
}

// Name returns the module name.
func (Scenes) Name() string {
	return "scenes"
}

// Register adds the scenes.
func (Scenes) Register(c *container.Container) error {
	for _, sc := range []scene.Scene{scene.Overview{}} {
		if err := c.Scenes.Add(sc.Name(), sc); err != nil {
			return err
		}
	}
	return nil
}
