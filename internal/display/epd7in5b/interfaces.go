package epd7in5b

import (
	"context"
	"image"

	epd "github.com/mshaulsky/waveshare-epd7in5b-v2"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=epd7in5b

// Panel is the driver surface the display drives: *epd7in5b.Device satisfies it.
type Panel interface {
	Init(ctx context.Context) error
	InitFast(ctx context.Context) error
	InitPartial(ctx context.Context) error
	Display(ctx context.Context, img image.Image) error
	DisplayPartial(ctx context.Context, img image.Image, r image.Rectangle) error
	Clear(ctx context.Context) error
	Sleep(ctx context.Context) error
}

var _ Panel = (*epd.Device)(nil)
