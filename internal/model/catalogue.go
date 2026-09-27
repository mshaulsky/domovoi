package model

// MetricInfo describes one metric of the catalogue: the kind of value it
// carries, its unit, and — for boolean metrics — the events its transitions
// produce. A zero OnRise or OnFall means the transition is not an event.
type MetricInfo struct {
	Kind   ValueKind
	Unit   string
	OnRise EventKind // false → true
	OnFall EventKind // true → false
}

// Metrics. A metric exists only once a scene or a rule consumes it; sources
// drop vendor points that map to nothing.
const (
	Temperature              Metric = "temperature"
	Humidity                 Metric = "humidity"
	Battery                  Metric = "battery"
	Leak                     Metric = "leak"
	Locked                   Metric = "locked"
	Power                    Metric = "power"
	TargetTemperature        Metric = "target_temperature"
	Heating                  Metric = "heating"
	PowerDraw                Metric = "power_draw"
	Online                   Metric = "online"
	TemperatureAlarm         Metric = "temperature_alarm"
	HumidityAlarm            Metric = "humidity_alarm"
	Note                     Metric = "note"
	WindSpeed                Metric = "wind_speed"
	PrecipitationProbability Metric = "precipitation_probability"
	WeatherCode              Metric = "weather_code"
	ForecastHigh             Metric = "forecast_high"
	ForecastLow              Metric = "forecast_low"
	Sunrise                  Metric = "sunrise"
	Sunset                   Metric = "sunset"
)

// catalogue is the one table describing every metric.
var catalogue = map[Metric]MetricInfo{
	Temperature:              {Kind: Number, Unit: "°C"},
	Humidity:                 {Kind: Number, Unit: "%"},
	Battery:                  {Kind: Number, Unit: "%"},
	Leak:                     {Kind: Bool, OnRise: EventLeakStarted, OnFall: EventLeakEnded},
	Locked:                   {Kind: Bool, OnRise: EventLocked, OnFall: EventUnlocked},
	Power:                    {Kind: Bool, OnRise: EventPoweredOn, OnFall: EventPoweredOff},
	TargetTemperature:        {Kind: Number, Unit: "°C"}, // a thermostat's setpoint
	Heating:                  {Kind: Bool},               // a thermostat's valve is open
	PowerDraw:                {Kind: Number, Unit: "W"},  // a metering outlet's current load
	Online:                   {Kind: Bool, OnRise: EventOnline, OnFall: EventOffline},
	TemperatureAlarm:         {Kind: Text},
	HumidityAlarm:            {Kind: Text},
	Note:                     {Kind: Text},
	WindSpeed:                {Kind: Number, Unit: "m/s"},
	PrecipitationProbability: {Kind: Number, Unit: "%"},
	WeatherCode:              {Kind: Number}, // WMO code
	ForecastHigh:             {Kind: Number, Unit: "°C"},
	ForecastLow:              {Kind: Number, Unit: "°C"},
	Sunrise:                  {Kind: Text}, // HH:MM local
	Sunset:                   {Kind: Text},
}

// Lookup returns the catalogue entry of a metric.
func Lookup(m Metric) (MetricInfo, bool) {
	info, ok := catalogue[m]
	return info, ok
}

// Unit returns the metric's unit, "" for unitless and unknown metrics.
func (m Metric) Unit() string {
	return catalogue[m].Unit
}

// Transition returns the event a change from old to now produces, if the
// metric is boolean and the change is a transition the catalogue names.
func (m Metric) Transition(old, now Value) (EventKind, bool) {
	info, ok := catalogue[m]
	if !ok || info.Kind != Bool || old.Kind != Bool || now.Kind != Bool {
		return "", false
	}
	switch {
	case !old.Bool() && now.Bool() && info.OnRise != "":
		return info.OnRise, true
	case old.Bool() && !now.Bool() && info.OnFall != "":
		return info.OnFall, true
	default:
		return "", false
	}
}
