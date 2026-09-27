package scene

import (
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/icon"
	"github.com/mshaulsky/domovoi/internal/model"
)

func TestViewReading(t *testing.T) {
	v := fixture(t, "en")
	if r, ok := v.Reading("tuya:office", model.Temperature); !ok || r.Value.Num != 22.4 {
		t.Errorf("Reading = %+v, %t", r, ok)
	}
	if _, ok := v.Reading("nope", model.Temperature); ok {
		t.Error("unknown device present")
	}
}

func TestViewNumber(t *testing.T) {
	v := fixture(t, "en")
	tests := []struct {
		name   string
		id     model.DeviceID
		metric model.Metric
		want   float64
		wantOK bool
	}{
		{name: "number", id: "tuya:office", metric: model.Humidity, want: 48, wantOK: true},
		{name: "bool is not a number", id: "aqara:lock", metric: model.Locked, wantOK: false},
		{name: "missing", id: "tuya:office", metric: model.Leak, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := v.Number(tt.id, tt.metric)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("Number = %v, %t; want %v, %t", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestViewBool(t *testing.T) {
	v := fixture(t, "en")
	if got, ok := v.Bool("aqara:leak1", model.Leak); !ok || !got {
		t.Errorf("Bool(leak1) = %t, %t", got, ok)
	}
	if _, ok := v.Bool("tuya:office", model.Temperature); ok {
		t.Error("number reported as bool")
	}
}

func TestViewText(t *testing.T) {
	v := fixture(t, "en")
	if got, ok := v.Text("weather:home", model.Sunrise); !ok || got != "06:41" {
		t.Errorf("Text(sunrise) = %q, %t", got, ok)
	}
	if _, ok := v.Text("weather:home", model.Temperature); ok {
		t.Error("number reported as text")
	}
}

func TestViewHas(t *testing.T) {
	v := fixture(t, "en")
	if !v.Has("aqara:lock", model.Locked) || v.Has("aqara:lock", model.Leak) {
		t.Error("Has is wrong")
	}
}

func TestViewOffline(t *testing.T) {
	v := fixture(t, "en")
	if !v.Offline("tuya:kitchen") || v.Offline("tuya:office") || v.Offline("nope") {
		t.Error("Offline is wrong")
	}
}

func TestViewIcon(t *testing.T) {
	v := fixture(t, "en")
	d := v.Devices[1]
	if got := v.Icon(d); got != icon.Thermometer {
		t.Errorf("default icon = %v", got)
	}
	v.Icons = map[model.DeviceID]icon.Icon{d.ID: icon.Desk}
	if got := v.Icon(d); got != icon.Desk {
		t.Errorf("override icon = %v", got)
	}
}

func TestViewIsOutside(t *testing.T) {
	v := fixture(t, "en")
	if !v.IsOutside(v.Devices[0]) || v.IsOutside(v.Devices[1]) {
		t.Error("IsOutside is wrong")
	}
}

func TestViewTrend(t *testing.T) {
	v := fixture(t, "en")
	tests := []struct {
		name string
		id   model.DeviceID
		want icon.Icon
	}{
		{name: "rising", id: "tuya:office", want: icon.TrendUp},
		{name: "falling", id: "tuya:hall", want: icon.TrendDown},
		{name: "flat", id: "tuya:kitchen", want: icon.TrendFlat},
		{name: "no data", id: "tuya:bedroom", want: icon.None},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Trend(tt.id, model.Temperature); got != tt.want {
				t.Errorf("Trend = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestViewDeviceName(t *testing.T) {
	v := fixture(t, "en")
	if got := v.DeviceName(model.Device{Name: "x", Room: "r"}); got != "x" {
		t.Errorf("name = %q", got)
	}
	if got := v.DeviceName(model.Device{Room: "r"}); got != "r" {
		t.Errorf("room fallback = %q", got)
	}
}

func TestViewClock(t *testing.T) {
	v := fixture(t, "en")
	if got := v.Clock(epoch); got != "17:05" {
		t.Errorf("Clock in Almaty = %q", got)
	}
	v.Location = nil
	if got := v.Clock(epoch); got != "12:05" {
		t.Errorf("Clock without location = %q", got)
	}
}

func TestViewFormat(t *testing.T) {
	if got := fixture(t, "ru").Format(22.44, 1); got != "22,4" {
		t.Errorf("Format = %q", got)
	}
}

func TestViewEventSubject(t *testing.T) {
	v := fixture(t, "en")
	tests := []struct {
		name  string
		event model.Event
		want  string
	}{
		{name: "known device", event: model.Event{Device: "aqara:lock"}, want: "Door lock (U200)"},
		{name: "detail", event: model.Event{Kind: model.EventAlertRaised, Detail: "leak"}, want: "leak"},
		{name: "unknown device", event: model.Event{Device: "x:y"}, want: "x:y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.eventSubject(tt.event); got != tt.want {
				t.Errorf("eventSubject = %q, want %q", got, tt.want)
			}
		})
	}
	_ = time.Second
}

func TestViewWhen(t *testing.T) {
	v := fixture(t, "ru") // Now is 17:05 on Sun 13 Sep in Almaty
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "today", t: epoch.Add(-2 * time.Hour), want: "15:05"},
		{name: "yesterday", t: epoch.Add(-26 * time.Hour), want: "сб 12 сен 15:05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.When(tt.t); got != tt.want {
				t.Errorf("When() = %q, want %q", got, tt.want)
			}
		})
	}
}
