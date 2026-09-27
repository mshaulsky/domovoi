package scene

import (
	"fmt"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"github.com/mshaulsky/domovoi/internal/icon"
)

// Fonts holds the parsed typefaces. One Fonts serves every renderer in the
// process: an sfnt.Font is safe for concurrent use.
type Fonts struct {
	text  *sfnt.Font
	bold  *sfnt.Font
	icons *sfnt.Font
}

// Faces hands out font.Face values for one render, cached by kind and size.
// A Faces belongs to a single goroutine: an opentype face carries a glyph
// buffer and x/image documents it as unsafe for concurrent use, so every
// render builds its own from the shared Fonts. Building a face costs
// microseconds; a frame needs a couple of dozen.
type Faces struct {
	fonts *Fonts
	cache map[faceKey]font.Face
}

type faceKind uint8

type faceKey struct {
	kind faceKind
	size int
}

const (
	faceText faceKind = iota
	faceBold
	faceIcon
)

var (
	defaultOnce  sync.Once
	defaultFonts *Fonts
	defaultErr   error
)

// LoadFonts parses the Go fonts and the icon font.
func LoadFonts() (*Fonts, error) {
	text, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("scene: parse regular font: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("scene: parse bold font: %w", err)
	}
	icons, err := icon.Font()
	if err != nil {
		return nil, err
	}
	return &Fonts{text: text, bold: bold, icons: icons}, nil
}

// DefaultFonts parses the fonts once for the process.
func DefaultFonts() (*Fonts, error) {
	defaultOnce.Do(func() {
		defaultFonts, defaultErr = LoadFonts()
	})
	return defaultFonts, defaultErr
}

// Faces starts a fresh face cache for one render.
func (f *Fonts) Faces() *Faces {
	return &Faces{fonts: f, cache: map[faceKey]font.Face{}}
}

// Text returns the regular face at size pixels.
func (f *Faces) Text(size int) font.Face {
	return f.face(faceText, size)
}

// Bold returns the bold face at size pixels.
func (f *Faces) Bold(size int) font.Face {
	return f.face(faceBold, size)
}

// Icon returns the icon face at size pixels.
func (f *Faces) Icon(size int) font.Face {
	return f.face(faceIcon, size)
}

func (f *Faces) face(kind faceKind, size int) font.Face {
	size = max(size, 1)
	key := faceKey{kind: kind, size: size}
	if face, ok := f.cache[key]; ok {
		return face
	}
	src := f.fonts.text
	switch kind {
	case faceBold:
		src = f.fonts.bold
	case faceIcon:
		src = f.fonts.icons
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		// Only malformed options fail here; the fallback keeps a frame
		// legible rather than empty.
		face = basicfont.Face7x13
	}
	f.cache[key] = face
	return face
}
