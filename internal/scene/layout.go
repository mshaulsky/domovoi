package scene

import "image"

// Split divides r into len(weights) cells along one axis, proportionally.
// The last cell absorbs rounding so the cells tile r exactly.
func Split(r image.Rectangle, weights []int, vertical bool) []image.Rectangle {
	if len(weights) == 0 {
		return nil
	}
	total := 0
	for _, w := range weights {
		total += max(w, 0)
	}
	cells := make([]image.Rectangle, len(weights))
	length := r.Dx()
	if vertical {
		length = r.Dy()
	}
	pos := 0
	for i, w := range weights {
		size := 0
		if total > 0 {
			size = length * max(w, 0) / total
		}
		if i == len(weights)-1 {
			size = length - pos
		}
		if vertical {
			cells[i] = image.Rect(r.Min.X, r.Min.Y+pos, r.Max.X, r.Min.Y+pos+size)
		} else {
			cells[i] = image.Rect(r.Min.X+pos, r.Min.Y, r.Min.X+pos+size, r.Max.Y)
		}
		pos += size
	}
	return cells
}

// Even divides r into n equal cells along one axis.
func Even(r image.Rectangle, n int, vertical bool) []image.Rectangle {
	if n <= 0 {
		return nil
	}
	weights := make([]int, n)
	for i := range weights {
		weights[i] = 1
	}
	return Split(r, weights, vertical)
}

// Grid divides r into rows×cols cells with a gap between them, row-major.
func Grid(r image.Rectangle, cols, rows, gap int) []image.Rectangle {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	cells := make([]image.Rectangle, 0, cols*rows)
	for _, row := range Even(r, rows, true) {
		for _, cell := range Even(row, cols, false) {
			cells = append(cells, shrink(cell, gap))
		}
	}
	return cells
}

// shrink insets a cell by half the gap on every side, so neighbours end up
// gap apart while the outer edge keeps half a gap of margin.
func shrink(r image.Rectangle, gap int) image.Rectangle {
	return r.Inset(gap / 2)
}

// TakeTop cuts a strip of height h off the top of r and returns it with
// the remainder.
func TakeTop(r image.Rectangle, h int) (strip, rest image.Rectangle) {
	h = min(max(h, 0), r.Dy())
	return image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h), image.Rect(r.Min.X, r.Min.Y+h, r.Max.X, r.Max.Y)
}

// TakeBottom cuts a strip of height h off the bottom of r.
func TakeBottom(r image.Rectangle, h int) (strip, rest image.Rectangle) {
	h = min(max(h, 0), r.Dy())
	return image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-h)
}

// TakeLeft cuts a strip of width w off the left of r.
func TakeLeft(r image.Rectangle, w int) (strip, rest image.Rectangle) {
	w = min(max(w, 0), r.Dx())
	return image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), image.Rect(r.Min.X+w, r.Min.Y, r.Max.X, r.Max.Y)
}

// TakeRight cuts a strip of width w off the right of r.
func TakeRight(r image.Rectangle, w int) (strip, rest image.Rectangle) {
	w = min(max(w, 0), r.Dx())
	return image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, r.Max.X-w, r.Max.Y)
}
