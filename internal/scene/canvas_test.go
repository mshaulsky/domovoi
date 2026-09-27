package scene

import (
	"image"
	"image/color"
	"testing"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/icon"
)

func newTestCanvas(t *testing.T, w, h int) *Canvas {
	t.Helper()
	fonts, err := DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCanvas(surface(w, h, false), fonts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// count returns how many pixels of the canvas have the colour.
func count(c *Canvas, col color.Color) int {
	idx := uint8(c.img.Palette.Index(col))
	n := 0
	for _, p := range c.img.Pix {
		if p == idx {
			n++
		}
	}
	return n
}

func TestNewCanvas(t *testing.T) {
	c := newTestCanvas(t, 20, 10)
	if got := count(c, display.White); got != 200 {
		t.Errorf("fresh canvas has %d paper pixels, want 200", got)
	}
	if c.Bounds() != image.Rect(0, 0, 20, 10) || c.Theme().Accent != display.Red || c.Faces() == nil {
		t.Errorf("canvas = bounds %v theme %+v", c.Bounds(), c.Theme())
	}
	if _, err := NewCanvas(display.Surface{Palette: display.Mono()[:1]}, c.faces.fonts); err == nil {
		t.Error("NewCanvas with one colour should fail")
	}
}

func TestCanvasFill(t *testing.T) {
	c := newTestCanvas(t, 20, 10)
	c.Fill(image.Rect(5, 5, 15, 30), display.Red) // partly outside: clipped, not panicking
	if got := count(c, display.Red); got != 50 {
		t.Errorf("filled %d pixels, want 50", got)
	}
}

func TestCanvasOutline(t *testing.T) {
	c := newTestCanvas(t, 20, 10)
	c.Outline(image.Rect(0, 0, 20, 10), 1, display.Black)
	if got := count(c, display.Black); got != 2*20+2*8 {
		t.Errorf("outline inked %d pixels, want 56", got)
	}
}

func TestCanvasHLine(t *testing.T) {
	c := newTestCanvas(t, 20, 10)
	c.HLine(2, 12, 4, 2, display.Black)
	if got := count(c, display.Black); got != 20 {
		t.Errorf("line inked %d pixels, want 20", got)
	}
}

func TestCanvasVLine(t *testing.T) {
	c := newTestCanvas(t, 20, 10)
	c.VLine(3, 1, 9, 1, display.Black)
	if got := count(c, display.Black); got != 8 {
		t.Errorf("line inked %d pixels, want 8", got)
	}
}

func TestCanvasText(t *testing.T) {
	c := newTestCanvas(t, 200, 40)
	face := c.Faces().Text(20)
	adv := c.Text("Спальня 22,4°", face, display.Black, 2, 28)
	if adv <= 0 || adv != c.TextWidth("Спальня 22,4°", face) {
		t.Errorf("advance = %d", adv)
	}
	if count(c, display.Black) == 0 {
		t.Error("text drew nothing")
	}
	before := count(c, display.Black)
	c.Text("", face, display.Black, 2, 28)
	if count(c, display.Black) != before {
		t.Error("empty string changed the canvas")
	}
}

func TestCanvasTextWidth(t *testing.T) {
	c := newTestCanvas(t, 10, 10)
	face := c.Faces().Text(20)
	if c.TextWidth("ab", face) <= c.TextWidth("a", face) || c.TextWidth("", face) != 0 {
		t.Error("TextWidth does not grow with the string")
	}
}

func TestCanvasTextIn(t *testing.T) {
	tests := []struct {
		name  string
		align Align
		// which half of the box holds ink
		wantLeft, wantRight bool
	}{
		{name: "left", align: AlignLeft, wantLeft: true},
		{name: "right", align: AlignRight, wantRight: true},
		{name: "center", align: AlignCenter, wantLeft: true, wantRight: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCanvas(t, 200, 30)
			c.TextIn("12", c.Faces().Text(16), display.Black, c.Bounds(), tt.align)
			left, right := false, false
			for y := range 30 {
				for x := range 200 {
					if c.img.ColorIndexAt(x, y) == 1 {
						if x < 100 {
							left = true
						} else {
							right = true
						}
					}
				}
			}
			if left != tt.wantLeft || right != tt.wantRight {
				t.Errorf("ink left=%t right=%t, want %t/%t", left, right, tt.wantLeft, tt.wantRight)
			}
		})
	}
	t.Run("clipped to the box", func(t *testing.T) {
		c := newTestCanvas(t, 200, 30)
		c.TextIn("a very long string that cannot fit", c.Faces().Text(20), display.Black, image.Rect(0, 0, 40, 30), AlignLeft)
		for y := range 30 {
			for x := 40; x < 200; x++ {
				if c.img.ColorIndexAt(x, y) != 0 {
					t.Fatalf("ink at (%d,%d) outside the box", x, y)
				}
			}
		}
	})
}

func TestCanvasFitText(t *testing.T) {
	c := newTestCanvas(t, 10, 10)
	wide := c.FitText("wide wide wide", false, 40, 8, 60)
	if got := c.TextWidth("wide wide wide", wide); got > 60 && wide != c.Faces().Text(8) {
		t.Errorf("FitText left width %d without reaching the minimum", got)
	}
	if got := c.FitText("x", true, 30, 8, 500); got != c.Faces().Bold(30) {
		t.Error("FitText shrank text that already fit")
	}
}

func TestCanvasIcon(t *testing.T) {
	c := newTestCanvas(t, 40, 40)
	c.Icon(icon.None, 24, display.Black, 0, 0)
	if count(c, display.Black) != 0 {
		t.Error("None drew something")
	}
	c.Icon(icon.Thermometer, 24, display.Red, 8, 8)
	if count(c, display.Red) == 0 {
		t.Error("icon drew nothing")
	}
	for y := range 40 {
		for x := range 40 {
			if c.img.ColorIndexAt(x, y) == 2 && (x < 8 || x >= 32 || y < 8 || y >= 32) {
				t.Fatalf("icon ink at (%d,%d) outside its box", x, y)
			}
		}
	}
}

func TestCanvasIconIn(t *testing.T) {
	c := newTestCanvas(t, 60, 30)
	c.IconIn(icon.Lock, display.Black, image.Rect(10, 0, 50, 30))
	if count(c, display.Black) == 0 {
		t.Error("IconIn drew nothing")
	}
}
