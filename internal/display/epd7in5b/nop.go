package epd7in5b

import (
	"context"
	"image"
)

// NopPanel is a Panel that does nothing: every call succeeds at once, like
// io.NopCloser for a panel. It is what -check builds the display against,
// so the configuration is validated end to end without opening SPI or GPIO.
type NopPanel struct{}

var _ Panel = NopPanel{}

// Init does nothing.
func (NopPanel) Init(context.Context) error {
	return nil
}

// InitFast does nothing.
func (NopPanel) InitFast(context.Context) error {
	return nil
}

// InitPartial does nothing.
func (NopPanel) InitPartial(context.Context) error {
	return nil
}

// Display does nothing.
func (NopPanel) Display(context.Context, image.Image) error {
	return nil
}

// DisplayPartial does nothing.
func (NopPanel) DisplayPartial(context.Context, image.Image, image.Rectangle) error {
	return nil
}

// Clear does nothing.
func (NopPanel) Clear(context.Context) error {
	return nil
}

// Sleep does nothing.
func (NopPanel) Sleep(context.Context) error {
	return nil
}
