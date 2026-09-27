package icon

import (
	"slices"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/mshaulsky/domovoi/internal/model"
)

func TestFont(t *testing.T) {
	f, err := Font()
	if err != nil {
		t.Fatal(err)
	}
	if f.NumGlyphs() < len(All()) {
		t.Errorf("font has %d glyphs, fewer than the %d icons", f.NumGlyphs(), len(All()))
	}
}

// TestAll is the guard against a stale subset: every constant must resolve
// to a real glyph, never .notdef.
func TestAll(t *testing.T) {
	f, err := Font()
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	all := All()
	if !slices.IsSorted(all) {
		t.Error("All() is not sorted")
	}
	for name, ic := range names {
		idx, err := f.GlyphIndex(&buf, rune(ic))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if idx == 0 {
			t.Errorf("%s (U+%X) is missing from the subset; run make icons", name, int(ic))
		}
	}
}

func TestFace(t *testing.T) {
	tests := []struct {
		name string
		size float64
	}{
		{name: "small", size: 16},
		{name: "large", size: 96},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			face, err := Face(tt.size)
			if err != nil {
				t.Fatal(err)
			}
			defer face.Close()
			_, adv, ok := face.GlyphBounds(rune(Thermometer))
			if !ok || adv <= 0 {
				t.Errorf("GlyphBounds(Thermometer) ok=%t advance=%v", ok, adv)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		want   Icon
		wantOK bool
	}{
		{name: "known", key: "bed", want: Bed, wantOK: true},
		{name: "unknown", key: "bathtub", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.key)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("Lookup(%q) = %v, %t; want %v, %t", tt.key, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestNames(t *testing.T) {
	got := Names()
	if !slices.IsSorted(got) || len(got) != len(names) {
		t.Errorf("Names() = %v", got)
	}
}

func TestForCondition(t *testing.T) {
	tests := []struct {
		name string
		cond model.WeatherCondition
		want Icon
	}{
		{name: "clear", cond: model.WeatherClear, want: WeatherSunny},
		{name: "rain", cond: model.WeatherRain, want: WeatherRainy},
		{name: "thunderstorm", cond: model.WeatherThunderstorm, want: WeatherLightningRainy},
		{name: "unknown", cond: model.WeatherUnknown, want: Help},
		{name: "unmapped", cond: "hurricane", want: Help},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForCondition(tt.cond); got != tt.want {
				t.Errorf("ForCondition(%q) = %v, want %v", tt.cond, got, tt.want)
			}
		})
	}
	for _, c := range []model.WeatherCondition{
		model.WeatherClear, model.WeatherMainlyClear, model.WeatherPartlyCloudy, model.WeatherOvercast,
		model.WeatherFog, model.WeatherDrizzle, model.WeatherRain, model.WeatherFreezingRain, model.WeatherSnow,
		model.WeatherShowers, model.WeatherSnowShowers, model.WeatherThunderstorm, model.WeatherUnknown,
	} {
		if _, ok := conditions[c]; !ok {
			t.Errorf("condition %q has no icon", c)
		}
	}
}

func TestForKind(t *testing.T) {
	tests := []struct {
		name string
		kind model.Kind
		want Icon
	}{
		{name: "lock", kind: model.KindLock, want: Lock},
		{name: "leak sensor", kind: model.KindLeakSensor, want: Water},
		{name: "unknown kind", kind: "toaster", want: Help},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForKind(tt.kind); got != tt.want {
				t.Errorf("ForKind(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestForBattery(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		want    Icon
	}{
		{name: "full", percent: 100, want: Battery},
		{name: "ninety-five", percent: 95, want: Battery},
		{name: "ninety-four", percent: 94, want: Battery90},
		{name: "half", percent: 50, want: Battery50},
		{name: "ten", percent: 10, want: Battery10},
		{name: "four", percent: 4, want: BatteryEmpty},
		{name: "zero", percent: 0, want: BatteryEmpty},
		{name: "above range clamps", percent: 140, want: Battery},
		{name: "unknown", percent: -1, want: BatteryUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForBattery(tt.percent); got != tt.want {
				t.Errorf("ForBattery(%d) = %v, want %v", tt.percent, got, tt.want)
			}
		})
	}
}

func TestForTrend(t *testing.T) {
	tests := []struct {
		name  string
		delta float64
		want  Icon
	}{
		{name: "rising", delta: 0.6, want: TrendUp},
		{name: "falling", delta: -0.6, want: TrendDown},
		{name: "flat", delta: 0.2, want: TrendFlat},
		{name: "exactly at band", delta: 0.5, want: TrendFlat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForTrend(tt.delta, 0.5); got != tt.want {
				t.Errorf("ForTrend(%g) = %v, want %v", tt.delta, got, tt.want)
			}
		})
	}
}

func TestIconString(t *testing.T) {
	if got := Thermometer.String(); got != "\U000F050F" {
		t.Errorf("String() = %q", got)
	}
}
