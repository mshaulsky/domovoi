package scene

import (
	"image"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/model"
)

// Overview is the first screen: alerts, header, the climate grid with the
// weather first, the hallway strip and the footer.
type Overview struct {
	Fonts *Fonts // nil = DefaultFonts
}

// Name returns the scene's config name.
func (Overview) Name() string {
	return "overview"
}

// Render draws the overview for the surface.
func (o Overview) Render(s display.Surface, v View) (*image.Paletted, error) {
	fonts := o.Fonts
	if fonts == nil {
		f, err := DefaultFonts()
		if err != nil {
			return nil, err
		}
		fonts = f
	}
	c, err := NewCanvas(s, fonts)
	if err != nil {
		return nil, err
	}
	sc := newScale(c.Bounds())
	th := c.Theme()
	r := c.Bounds()

	if len(v.Alerts) > 0 {
		var bar image.Rectangle
		bar, r = TakeTop(r, sc.px(alertHeight))
		AlertBar{}.Draw(c, bar, v)
	}
	header, r := TakeTop(r, sc.px(headerHeight))
	Header{}.Draw(c, header, v)
	c.HLine(r.Min.X, r.Max.X, r.Min.Y, sc.px(lineWidth), th.Ink)

	if !v.Starting && (v.Note != "" || len(v.Events) > 0) {
		var footer image.Rectangle
		footer, r = TakeBottom(r, sc.px(footerHeight))
		c.HLine(footer.Min.X, footer.Max.X, footer.Min.Y, sc.px(lineWidth), th.Ink)
		Footer{}.Draw(c, footer, v)
	}

	hall := hallwayDevices(v)
	if len(hall) > 0 {
		var strip image.Rectangle
		strip, r = TakeBottom(r, hallwayHeight(r.Dx(), sc, len(hall)))
		c.HLine(strip.Min.X, strip.Max.X, strip.Min.Y, sc.px(lineWidth), th.Ink)
		Hallway{Devices: hall}.Draw(c, strip, v)
	}

	tiles := tileDevices(v)
	if len(tiles) == 0 {
		key := "scene.no_devices"
		if v.Starting {
			key = "scene.starting"
		}
		c.TextIn(v.Bundle.T(key), c.Faces().Text(sc.px(24)), th.Ink, r, AlignCenter)
		return c.Image(), nil
	}
	cols := tileColumns(len(tiles))
	rows := (len(tiles) + cols - 1) / cols
	cells := Grid(r.Inset(sc.px(tileGap)/2), cols, rows, sc.px(tileGap))
	for i, d := range tiles {
		if v.IsOutside(d) {
			OutsideTile{Device: d}.Draw(c, cells[i], v)
			continue
		}
		ClimateTile{Device: d}.Draw(c, cells[i], v)
	}
	return c.Image(), nil
}

// tileDevices lists the devices that get a tile: the weather first, then
// everything with a temperature.
func tileDevices(v View) []model.Device {
	var outside, rest []model.Device
	for _, d := range v.Devices {
		switch {
		case v.IsOutside(d):
			outside = append(outside, d)
		case v.Has(d.ID, model.Temperature):
			rest = append(rest, d)
		}
	}
	return append(outside, rest...)
}

// tileColumns picks the grid width for n tiles: two columns up to four
// tiles, three up to six, four beyond. Tiles are designed for the 4×2 grid
// on 800×480, so they never shrink there; rows are the scarce side, since
// a third row would cut a tile's height by a third.
func tileColumns(n int) int {
	switch {
	case n > 6:
		return 4
	case n > 4:
		return 3
	default:
		return 2
	}
}

// hallwayDevices lists the devices with a lock, leak or power state that
// have no tile of their own: a thermostat's switch belongs to its tile.
func hallwayDevices(v View) []model.Device {
	var out []model.Device
	for _, d := range v.Devices {
		if v.IsOutside(d) || v.Has(d.ID, model.Temperature) {
			continue
		}
		if v.Has(d.ID, model.Locked) || v.Has(d.ID, model.Leak) || v.Has(d.ID, model.Power) {
			out = append(out, d)
		}
	}
	return out
}
