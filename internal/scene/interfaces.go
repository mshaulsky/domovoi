package scene

import (
	"image"

	"github.com/mshaulsky/domovoi/internal/display"
)

// Scene renders a view onto a surface.
type Scene interface {
	Name() string
	Render(s display.Surface, v View) (*image.Paletted, error)
}
