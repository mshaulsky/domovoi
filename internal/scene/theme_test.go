package scene

import (
	"image/color"
	"testing"

	"github.com/mshaulsky/domovoi/internal/display"
)

func TestThemeFor(t *testing.T) {
	grey := color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}
	tests := []struct {
		name    string
		palette color.Palette
		want    Theme
		wantErr bool
	}{
		{name: "three colour", palette: display.ThreeColour(), want: Theme{Paper: display.White, Ink: display.Black, Accent: display.Red}},
		{name: "mono", palette: display.Mono(), want: Theme{Paper: display.White, Ink: display.Black, Accent: display.Black}},
		{name: "order does not matter", palette: color.Palette{display.Red, display.Black, display.White}, want: Theme{Paper: display.White, Ink: display.Black, Accent: display.Red}},
		{name: "grey is not an accent", palette: color.Palette{display.White, grey, display.Black}, want: Theme{Paper: display.White, Ink: display.Black, Accent: display.Black}},
		{name: "too few colours", palette: color.Palette{display.White}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ThemeFor(tt.palette)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ThemeFor = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestThemeHasAccent(t *testing.T) {
	colour, _ := ThemeFor(display.ThreeColour())
	mono, _ := ThemeFor(display.Mono())
	if !colour.HasAccent() || mono.HasAccent() {
		t.Errorf("HasAccent: colour=%t mono=%t", colour.HasAccent(), mono.HasAccent())
	}
}
