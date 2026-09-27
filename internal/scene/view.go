package scene

import (
	"time"

	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/icon"
	"github.com/mshaulsky/domovoi/internal/model"
)

// View is everything a scene may draw: plain data assembled by the render
// coordinator, so scenes stay pure and testable.
type View struct {
	Now      time.Time
	Location *time.Location
	Bundle   *i18n.Bundle
	Devices  []model.Device // visible devices, in display order
	Readings map[model.DeviceID]map[model.Metric]model.Reading
	Stale    map[model.DeviceID]bool      // the device's data is not current: source or device silent
	Seen     map[model.DeviceID]time.Time // when the device last delivered any reading
	Sources  []SourceStatus
	Extremes map[model.DeviceID]map[model.Metric]Extremes // today's, when history exists
	Trends   map[model.DeviceID]map[model.Metric]float64  // change over the trend window
	Alerts   []Alert
	Events   []model.Event                // most recent first
	Icons    map[model.DeviceID]icon.Icon // overrides from settings
	Note     string                       // the advisor's line, when there is one
	Starting bool                         // no source has delivered anything yet
}

// SourceStatus is one source's freshness for the header.
type SourceStatus struct {
	Name   string
	LastOK time.Time // zero = never
	Stale  bool
}

// Extremes are a metric's minimum and maximum over a period.
type Extremes struct {
	Min, Max     float64
	MinAt, MaxAt time.Time
}

// Alert is an active alert as the scene shows it.
type Alert struct {
	Severity string // urgent, warning, info
	Message  string
	Since    time.Time
}

// Alert severities, as the alert engine names them.
const (
	SeverityUrgent  = "urgent"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// trendDeadBand is the change below which a trend counts as flat.
const trendDeadBand = 0.3

// Reading returns the latest reading of a metric.
func (v View) Reading(id model.DeviceID, m model.Metric) (model.Reading, bool) {
	r, ok := v.Readings[id][m]
	return r, ok
}

// Number returns a Number reading's value.
func (v View) Number(id model.DeviceID, m model.Metric) (float64, bool) {
	r, ok := v.Readings[id][m]
	if !ok || r.Value.Kind != model.Number {
		return 0, false
	}
	return r.Value.Num, true
}

// Bool returns a Bool reading's value.
func (v View) Bool(id model.DeviceID, m model.Metric) (val, ok bool) {
	r, ok := v.Readings[id][m]
	if !ok || r.Value.Kind != model.Bool {
		return false, false
	}
	return r.Value.Bool(), true
}

// Text returns a Text reading's value.
func (v View) Text(id model.DeviceID, m model.Metric) (string, bool) {
	r, ok := v.Readings[id][m]
	if !ok || r.Value.Kind != model.Text {
		return "", false
	}
	return r.Value.Text, true
}

// Has reports whether the device has a reading of the metric.
func (v View) Has(id model.DeviceID, m model.Metric) bool {
	_, ok := v.Readings[id][m]
	return ok
}

// Offline reports whether the device's own online flag says it is away.
func (v View) Offline(id model.DeviceID) bool {
	online, ok := v.Bool(id, model.Online)
	return ok && !online
}

// Icon returns the device's icon: the override, or the default of its kind.
func (v View) Icon(d model.Device) icon.Icon {
	if ic, ok := v.Icons[d.ID]; ok {
		return ic
	}
	return icon.ForKind(d.Kind)
}

// IsOutside reports whether a device is the weather: a virtual device
// carrying a weather code.
func (v View) IsOutside(d model.Device) bool {
	return d.Kind == model.KindVirtual && v.Has(d.ID, model.WeatherCode)
}

// Trend returns the trend arrow of a metric, or None without trend data.
func (v View) Trend(id model.DeviceID, m model.Metric) icon.Icon {
	delta, ok := v.Trends[id][m]
	if !ok {
		return icon.None
	}
	return icon.ForTrend(delta, trendDeadBand)
}

// DeviceName returns the device's display name; the room when the vendor
// gave the sensor the room's name, as Tuya climate sensors tend to have.
func (v View) DeviceName(d model.Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Room
}

// Clock formats a moment in the view's location as HH:MM.
func (v View) Clock(t time.Time) string {
	if v.Location != nil {
		t = t.In(v.Location)
	}
	return v.Bundle.Time(t)
}

// When formats a moment for a "since" note: the clock when it is today in
// the view's zone, the date and clock otherwise.
func (v View) When(t time.Time) string {
	now := v.Now
	if v.Location != nil {
		t, now = t.In(v.Location), now.In(v.Location)
	}
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return v.Bundle.Time(t)
	}
	return v.Bundle.Date(t) + " " + v.Bundle.Time(t)
}

// Format formats a number for the view's language.
func (v View) Format(f float64, decimals int) string {
	return v.Bundle.Number(f, decimals)
}
