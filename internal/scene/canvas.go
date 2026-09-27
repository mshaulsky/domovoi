package scene

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/icon"
)

// Canvas is a paletted image with a theme and fonts, and the drawing
// primitives widgets use. Text and icons are rasterised into a coverage mask
// and thresholded, so a 1-bit panel gets clean edges instead of a nearest-
// colour guess per anti-aliased pixel.
type Canvas struct {
	img   *image.Paletted
	theme Theme
	faces *Faces
}

// Align positions text horizontally inside a rectangle.
type Align int

// Alignments.
const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// coverage is the mask threshold: half-covered pixels and above are inked.
const coverage = 128

// NewCanvas prepares a frame filled with paper.
func NewCanvas(s display.Surface, fonts *Fonts) (*Canvas, error) {
	theme, err := ThemeFor(s.Palette)
	if err != nil {
		return nil, err
	}
	img := image.NewPaletted(s.Bounds, s.Palette)
	c := &Canvas{img: img, theme: theme, faces: fonts.Faces()}
	c.Fill(s.Bounds, theme.Paper)
	return c, nil
}

// Image returns the frame.
func (c *Canvas) Image() *image.Paletted {
	return c.img
}

// Bounds returns the drawable area.
func (c *Canvas) Bounds() image.Rectangle {
	return c.img.Bounds()
}

// Theme returns the semantic colours.
func (c *Canvas) Theme() Theme {
	return c.theme
}

// Faces returns this render's face cache.
func (c *Canvas) Faces() *Faces {
	return c.faces
}

// Fill paints a rectangle.
func (c *Canvas) Fill(r image.Rectangle, col color.Color) {
	draw.Draw(c.img, r.Intersect(c.img.Bounds()), image.NewUniform(col), image.Point{}, draw.Src)
}

// Outline strokes a rectangle's border on the inside.
func (c *Canvas) Outline(r image.Rectangle, thickness int, col color.Color) {
	c.Fill(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+thickness), col)
	c.Fill(image.Rect(r.Min.X, r.Max.Y-thickness, r.Max.X, r.Max.Y), col)
	c.Fill(image.Rect(r.Min.X, r.Min.Y, r.Min.X+thickness, r.Max.Y), col)
	c.Fill(image.Rect(r.Max.X-thickness, r.Min.Y, r.Max.X, r.Max.Y), col)
}

// HLine draws a horizontal line from x0 to x1 at y.
func (c *Canvas) HLine(x0, x1, y, thickness int, col color.Color) {
	c.Fill(image.Rect(x0, y, x1, y+thickness), col)
}

// VLine draws a vertical line from y0 to y1 at x.
func (c *Canvas) VLine(x, y0, y1, thickness int, col color.Color) {
	c.Fill(image.Rect(x, y0, x+thickness, y1), col)
}

// Text draws s with its baseline starting at (x, y) and returns the advance.
func (c *Canvas) Text(s string, face font.Face, col color.Color, x, y int) int {
	c.stamp(face, s, col, fixed.P(x, y))
	return c.TextWidth(s, face)
}

// TextWidth measures s.
func (c *Canvas) TextWidth(s string, face font.Face) int {
	return font.MeasureString(face, s).Ceil()
}

// TextIn draws s inside r, vertically centred on the cap height,
// horizontally aligned and clipped to r. Nothing wraps — a dashboard shows
// short strings; use FitText to shrink one that is too wide.
func (c *Canvas) TextIn(s string, face font.Face, col color.Color, r image.Rectangle, align Align) {
	w := c.TextWidth(s, face)
	x := r.Min.X
	switch align {
	case AlignCenter:
		x = r.Min.X + (r.Dx()-w)/2
	case AlignRight:
		x = r.Max.X - w
	}
	c.stampIn(face, s, col, fixed.P(x, baseline(face, r)), r)
}

// FitText returns the largest face between size and minSize at which s
// fits in maxWidth; the caller clips what still does not fit.
func (c *Canvas) FitText(s string, bold bool, size, minSize, maxWidth int) font.Face {
	for sz := size; sz > minSize; sz-- {
		face := c.faces.Text(sz)
		if bold {
			face = c.faces.Bold(sz)
		}
		if c.TextWidth(s, face) <= maxWidth {
			return face
		}
	}
	if bold {
		return c.faces.Bold(minSize)
	}
	return c.faces.Text(minSize)
}

// Icon draws an icon glyph centred in the size×size box at (x, y).
func (c *Canvas) Icon(ic icon.Icon, size int, col color.Color, x, y int) {
	if ic == icon.None {
		return
	}
	face := c.faces.Icon(size)
	bounds, _, ok := face.GlyphBounds(rune(ic))
	if !ok {
		return
	}
	gw := (bounds.Max.X - bounds.Min.X).Ceil()
	gh := (bounds.Max.Y - bounds.Min.Y).Ceil()
	dot := fixed.P(x+(size-gw)/2, y+(size-gh)/2)
	dot.X -= bounds.Min.X
	dot.Y -= bounds.Min.Y
	c.stamp(face, ic.String(), col, dot)
}

// IconIn draws an icon centred in r at the largest size that fits.
func (c *Canvas) IconIn(ic icon.Icon, col color.Color, r image.Rectangle) {
	size := min(r.Dx(), r.Dy())
	c.Icon(ic, size, col, r.Min.X+(r.Dx()-size)/2, r.Min.Y+(r.Dy()-size)/2)
}

// stamp rasterises s through a coverage mask and inks the covered pixels.
func (c *Canvas) stamp(face font.Face, s string, col color.Color, dot fixed.Point26_6) {
	c.stampIn(face, s, col, dot, c.img.Bounds())
}

// stampIn is stamp clipped to a rectangle.
func (c *Canvas) stampIn(face font.Face, s string, col color.Color, dot fixed.Point26_6, clip image.Rectangle) {
	if s == "" {
		return
	}
	bounds, _ := font.BoundString(face, s)
	r := image.Rect(
		(dot.X + bounds.Min.X).Floor(), (dot.Y + bounds.Min.Y).Floor(),
		(dot.X+bounds.Max.X).Ceil()+1, (dot.Y+bounds.Max.Y).Ceil()+1,
	)
	visible := r.Intersect(c.img.Bounds()).Intersect(clip)
	if visible.Empty() {
		return
	}
	mask := image.NewAlpha(r)
	d := font.Drawer{Dst: mask, Src: image.NewUniform(color.Alpha{A: 0xFF}), Face: face, Dot: dot}
	d.DrawString(s)
	idx := uint8(c.img.Palette.Index(col))
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		for x := visible.Min.X; x < visible.Max.X; x++ {
			if mask.AlphaAt(x, y).A >= coverage {
				c.img.SetColorIndex(x, y, idx)
			}
		}
	}
}

// baseline returns the y of a baseline that centres capitals in r.
func baseline(face font.Face, r image.Rectangle) int {
	m := face.Metrics()
	capHeight := m.CapHeight.Ceil()
	if capHeight == 0 {
		capHeight = m.Ascent.Ceil()
	}
	return r.Min.Y + (r.Dy()+capHeight)/2
}
