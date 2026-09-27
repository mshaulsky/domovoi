package scene

import (
	"image"
	"testing"
)

var box = image.Rect(10, 20, 110, 80) // 100×60

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		weights  []int
		vertical bool
		want     []image.Rectangle
	}{
		{name: "none", weights: nil, want: nil},
		{name: "horizontal thirds absorb rounding", weights: []int{1, 1, 1}, want: []image.Rectangle{
			image.Rect(10, 20, 43, 80), image.Rect(43, 20, 76, 80), image.Rect(76, 20, 110, 80),
		}},
		{name: "vertical weighted", weights: []int{1, 2}, vertical: true, want: []image.Rectangle{
			image.Rect(10, 20, 110, 40), image.Rect(10, 40, 110, 80),
		}},
		{name: "zero weight takes nothing", weights: []int{0, 1}, want: []image.Rectangle{
			image.Rect(10, 20, 10, 80), image.Rect(10, 20, 110, 80),
		}},
		{name: "all zero gives the last everything", weights: []int{0, 0}, want: []image.Rectangle{
			image.Rect(10, 20, 10, 80), image.Rect(10, 20, 110, 80),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Split(box, tt.weights, tt.vertical)
			if len(got) != len(tt.want) {
				t.Fatalf("Split = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("cell %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEven(t *testing.T) {
	if got := Even(box, 0, false); got != nil {
		t.Errorf("Even(0) = %v", got)
	}
	got := Even(box, 4, true)
	if len(got) != 4 || got[0].Dy() != 15 || got[3].Max.Y != 80 {
		t.Errorf("Even(4) = %v", got)
	}
}

func TestGrid(t *testing.T) {
	if got := Grid(box, 0, 1, 0); got != nil {
		t.Errorf("Grid(0 cols) = %v", got)
	}
	cells := Grid(box, 2, 2, 4)
	if len(cells) != 4 {
		t.Fatalf("cells = %d", len(cells))
	}
	if cells[0] != image.Rect(12, 22, 58, 48) || cells[3] != image.Rect(62, 52, 108, 78) {
		t.Errorf("cells = %v", cells)
	}
}

func TestTakeTop(t *testing.T) {
	strip, rest := TakeTop(box, 10)
	if strip != image.Rect(10, 20, 110, 30) || rest != image.Rect(10, 30, 110, 80) {
		t.Errorf("TakeTop = %v, %v", strip, rest)
	}
	if strip, rest := TakeTop(box, 999); strip != box || !rest.Empty() {
		t.Errorf("TakeTop(too much) = %v, %v", strip, rest)
	}
}

func TestTakeBottom(t *testing.T) {
	strip, rest := TakeBottom(box, 10)
	if strip != image.Rect(10, 70, 110, 80) || rest != image.Rect(10, 20, 110, 70) {
		t.Errorf("TakeBottom = %v, %v", strip, rest)
	}
}

func TestTakeLeft(t *testing.T) {
	strip, rest := TakeLeft(box, 30)
	if strip != image.Rect(10, 20, 40, 80) || rest != image.Rect(40, 20, 110, 80) {
		t.Errorf("TakeLeft = %v, %v", strip, rest)
	}
}

func TestTakeRight(t *testing.T) {
	strip, rest := TakeRight(box, 30)
	if strip != image.Rect(80, 20, 110, 80) || rest != image.Rect(10, 20, 80, 80) {
		t.Errorf("TakeRight = %v, %v", strip, rest)
	}
	if strip, _ := TakeRight(box, -5); !strip.Empty() {
		t.Errorf("TakeRight(negative) = %v", strip)
	}
}
