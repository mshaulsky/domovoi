package scene

import (
	"errors"
	"image/color"
)

// Theme maps the semantic colours onto a palette: Paper is the background,
// Ink the text, Accent the alarm colour — red on a three-colour panel, ink
// on a mono one, so an accent never disappears.
type Theme struct {
	Paper  color.Color
	Ink    color.Color
	Accent color.Color
}

// ThemeFor derives a theme from a palette: the lightest colour is paper,
// the darkest ink, the most saturated of the rest accent.
func ThemeFor(p color.Palette) (Theme, error) {
	if len(p) < 2 {
		return Theme{}, errors.New("scene: a palette needs at least two colours")
	}
	paper, ink := p[0], p[0]
	for _, c := range p[1:] {
		if luminance(c) > luminance(paper) {
			paper = c
		}
		if luminance(c) < luminance(ink) {
			ink = c
		}
	}
	accent := ink
	best := 0.0
	for _, c := range p {
		if c == paper || c == ink {
			continue
		}
		if s := saturation(c); s > best {
			best, accent = s, c
		}
	}
	return Theme{Paper: paper, Ink: ink, Accent: accent}, nil
}

// HasAccent reports whether the accent is a colour of its own.
func (t Theme) HasAccent() bool {
	return t.Accent != t.Ink
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

func saturation(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	hi := max(r, g, b)
	lo := min(r, g, b)
	if hi == 0 {
		return 0
	}
	return float64(hi-lo) / float64(hi)
}
