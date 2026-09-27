package application

import (
	"github.com/mshaulsky/domovoi/internal/application/modules"
	"github.com/mshaulsky/domovoi/internal/container"
)

// enabled lists the compiled-in modules. The order is the lifecycle order:
// Start runs top-down, Stop bottom-up. Adding a capability is one line here
// plus its glue file in the modules package.
func enabled() []container.Module {
	return []container.Module{
		modules.Tuya{},
		modules.Aqara{},
		modules.Weather{},
		modules.PNGFile{},
		modules.Scenes{},
		&modules.Storage{},
		&modules.Core{},
	}
}
