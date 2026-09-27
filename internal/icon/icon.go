package icon

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

//go:embed mdi-subset.ttf
var subset []byte

var (
	parseOnce sync.Once
	parsed    *sfnt.Font
	parseErr  error
)

// Font returns the parsed icon font.
func Font() (*sfnt.Font, error) {
	parseOnce.Do(func() {
		parsed, parseErr = opentype.Parse(subset)
		if parseErr != nil {
			parseErr = fmt.Errorf("icon: parse font: %w", parseErr)
		}
	})
	return parsed, parseErr
}

// Face returns a face rendering icons at size pixels, hinted for crisp
// edges on 1-bit displays. Faces are cheap; callers may cache by size.
func Face(size float64) (font.Face, error) {
	f, err := Font()
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("icon: face %g: %w", size, err)
	}
	return face, nil
}

// All lists every icon constant, for the subset tool and the coverage test.
func All() []Icon {
	return slices.Compact(slices.Sorted(maps.Values(names)))
}
