package modules

import (
	"fmt"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/display/pngfile"
)

// PNGFile provides the "pngfile" display kind.
type PNGFile struct {
	container.Passive
}

// pngfileSection is the YAML shape of a pngfile display's options.
type pngfileSection struct {
	Path   string `yaml:"path"`
	Width  int    `yaml:"width"`
	Height int    `yaml:"height"`
	Mono   bool   `yaml:"mono"`
}

// Name returns the module name.
func (PNGFile) Name() string {
	return "pngfile"
}

// Register adds the constructor.
func (PNGFile) Register(c *container.Container) error {
	return c.Displays.Add("pngfile", func(cfg config.DisplaySection) (display.Display, error) {
		var sec pngfileSection
		if err := cfg.Options.Decode(&sec); err != nil {
			return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
		}
		d, err := pngfile.New(pngfile.Config{Path: sec.Path, Width: sec.Width, Height: sec.Height, Mono: sec.Mono})
		if err != nil {
			return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
		}
		return d, nil
	})
}
