package scene

import (
	"image"
	"testing"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/model"
)

// TestWidgetsDraw is the smoke test for every widget: each draws something
// inside its rectangle and nothing outside it. Appearance is covered by the
// overview goldens.
func TestWidgetsDraw(t *testing.T) {
	v := fixture(t, "ru")
	tests := []struct {
		name   string
		widget Widget
		rect   image.Rectangle
	}{
		{name: "header", widget: Header{}, rect: image.Rect(0, 0, 800, 44)},
		{name: "alert bar", widget: AlertBar{}, rect: image.Rect(0, 0, 800, 40)},
		{name: "climate tile", widget: ClimateTile{Device: v.Devices[1]}, rect: image.Rect(10, 10, 270, 160)},
		{name: "climate tile tiny", widget: ClimateTile{Device: v.Devices[1]}, rect: image.Rect(10, 10, 90, 60)},
		{name: "outside tile", widget: OutsideTile{Device: v.Devices[0]}, rect: image.Rect(10, 10, 270, 160)},
		{name: "hallway", widget: Hallway{Devices: v.Devices[5:]}, rect: image.Rect(0, 0, 800, 84)},
		{name: "hallway narrow", widget: Hallway{Devices: v.Devices[5:]}, rect: image.Rect(0, 0, 300, 170)},
		{name: "footer", widget: Footer{}, rect: image.Rect(0, 0, 800, 40)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCanvas(t, 800, 480)
			tt.widget.Draw(c, tt.rect, v)
			inked := 0
			for y := range 480 {
				for x := range 800 {
					if c.img.ColorIndexAt(x, y) == 0 {
						continue
					}
					if !image.Pt(x, y).In(tt.rect) {
						t.Fatalf("ink at (%d,%d) outside %v", x, y, tt.rect)
					}
					inked++
				}
			}
			if inked == 0 {
				t.Error("widget drew nothing")
			}
		})
	}
}

func TestWidgetsDrawEmpty(t *testing.T) {
	v := fixture(t, "en")
	v.Alerts, v.Events = nil, nil
	tests := []struct {
		name   string
		widget Widget
	}{
		{name: "alert bar without alerts", widget: AlertBar{}},
		{name: "hallway without devices", widget: Hallway{}},
		{name: "footer without events", widget: Footer{}},
		{name: "footer while starting", widget: startingFooter{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCanvas(t, 100, 40)
			tt.widget.Draw(c, c.Bounds(), v)
			if count(c, display.Black)+count(c, display.Red) != 0 {
				t.Error("widget drew with nothing to show")
			}
		})
	}
}

// startingFooter draws the footer with Starting set.
type startingFooter struct{}

func (startingFooter) Draw(c *Canvas, r image.Rectangle, v View) {
	v.Starting = true
	v.Events = []model.Event{{Device: "aqara:lock", Kind: model.EventUnlocked}}
	Footer{}.Draw(c, r, v)
}

func TestHallwayState(t *testing.T) {
	v := fixture(t, "en")
	tests := []struct {
		name      string
		device    model.DeviceID
		wantState string
		wantAlarm bool
	}{
		{name: "unlocked", device: "aqara:lock", wantState: "UNLOCKED", wantAlarm: true},
		{name: "leak", device: "aqara:leak1", wantState: "LEAK", wantAlarm: true},
		{name: "dry", device: "aqara:leak2", wantState: "dry"},
		{name: "outlet on", device: "aqara:washer", wantState: "on"},
		{name: "metering outlet on", device: "tuya:aquarium", wantState: "on · 28 W"},
		{name: "nothing to say", device: "tuya:office", wantState: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d model.Device
			for _, dev := range v.Devices {
				if dev.ID == tt.device {
					d = dev
				}
			}
			_, state, alarm := hallwayState(v, d)
			if state != tt.wantState || alarm != tt.wantAlarm {
				t.Errorf("hallwayState = %q, %t; want %q, %t", state, alarm, tt.wantState, tt.wantAlarm)
			}
		})
	}
}

func TestHallwayHeight(t *testing.T) {
	s := newScale(image.Rect(0, 0, 800, 480))
	tests := []struct {
		name     string
		width, n int
		want     int
	}{
		{name: "roomy keeps the design height", width: 800, n: 4, want: 84},
		{name: "six cells shrink the strip", width: 800, n: 6, want: 59},
		{name: "crowded stops at the floor", width: 800, n: 12, want: 46},
		{name: "one device", width: 300, n: 1, want: 84},
		{name: "no devices", width: 800, n: 0, want: 84},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hallwayHeight(tt.width, s, tt.n); got != tt.want {
				t.Errorf("hallwayHeight = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCapitalise(t *testing.T) {
	tests := []struct{ in, want string }{{"tuya", "Tuya"}, {"aqara", "Aqara"}, {"", ""}, {"ёж", "Ёж"}}
	for _, tt := range tests {
		if got := capitalise(tt.in); got != tt.want {
			t.Errorf("capitalise(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
