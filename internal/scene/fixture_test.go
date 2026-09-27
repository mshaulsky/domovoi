package scene

import (
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/model"
)

var (
	epoch  = time.Date(2026, 9, 13, 12, 5, 0, 0, time.UTC)
	almaty = time.FixedZone("Asia/Almaty", 5*3600)
)

// fixture builds a view with the user's real device inventory, a weather
// device, one alert and two events — every widget gets exercised.
func fixture(t *testing.T, lang string) View {
	t.Helper()
	bundle, err := i18n.Load(lang)
	if err != nil {
		t.Fatal(err)
	}
	devices := []model.Device{
		{ID: "weather:home", Name: "Home", Kind: model.KindVirtual},
		{ID: "tuya:office", Name: "кабинет", Room: "", Kind: model.KindClimateSensor},
		{ID: "tuya:bedroom", Name: "спальня", Kind: model.KindClimateSensor},
		{ID: "tuya:hall", Name: "Зал", Kind: model.KindClimateSensor},
		{ID: "tuya:kitchen", Name: "Кухня", Kind: model.KindClimateSensor},
		{ID: "tuya:thermostat", Name: "Thermostat", Kind: model.KindThermostat},
		{ID: "tuya:aquarium", Name: "Розетка Аквариум", Kind: model.KindOutlet},
		{ID: "aqara:lock", Name: "Дверной замок (U200)", Room: "Прихожая", Kind: model.KindLock},
		{ID: "aqara:washer", Name: "Розетка стиралка", Room: "Прихожая", Kind: model.KindOutlet},
		{ID: "aqara:leak1", Name: "Протечка стиралки", Room: "Прихожая", Kind: model.KindLeakSensor},
		{ID: "aqara:leak2", Name: "Протечка ванная", Room: "Ванная", Kind: model.KindLeakSensor},
	}
	num := model.NumberValue
	readings := map[model.DeviceID]map[model.Metric]model.Value{
		"weather:home": {
			model.Temperature: num(17.3), model.Humidity: num(62), model.WeatherCode: num(61),
			model.PrecipitationProbability: num(40), model.WindSpeed: num(4), model.ForecastHigh: num(21),
			model.ForecastLow: num(9), model.Sunrise: model.TextValue("06:41"), model.Sunset: model.TextValue("19:12"),
			model.Online: model.BoolValue(true),
		},
		"tuya:office":  {model.Temperature: num(22.4), model.Humidity: num(48), model.Battery: num(87), model.Online: model.BoolValue(true)},
		"tuya:bedroom": {model.Temperature: num(21.0), model.Humidity: num(51), model.Battery: num(12), model.Online: model.BoolValue(true)},
		"tuya:hall":    {model.Temperature: num(23.9), model.Humidity: num(44), model.Battery: num(64), model.Online: model.BoolValue(true)},
		"tuya:kitchen": {model.Temperature: num(19.5), model.Humidity: num(58), model.Battery: num(90), model.Online: model.BoolValue(false)},
		"tuya:thermostat": {
			model.Temperature: num(19.0), model.TargetTemperature: num(21.0),
			model.Power: model.BoolValue(true), model.Heating: model.BoolValue(true), model.Online: model.BoolValue(true),
		},
		"tuya:aquarium": {model.Power: model.BoolValue(true), model.PowerDraw: num(28.4), model.Online: model.BoolValue(true)},
		"aqara:lock":    {model.Locked: model.BoolValue(false), model.Online: model.BoolValue(true)},
		"aqara:washer":  {model.Power: model.BoolValue(true), model.Online: model.BoolValue(true)},
		"aqara:leak1":   {model.Leak: model.BoolValue(true), model.Online: model.BoolValue(true)},
		"aqara:leak2":   {model.Leak: model.BoolValue(false), model.Online: model.BoolValue(true)},
	}
	v := View{
		Now:      epoch,
		Location: almaty,
		Bundle:   bundle,
		Devices:  devices,
		Readings: map[model.DeviceID]map[model.Metric]model.Reading{},
		Stale:    map[model.DeviceID]bool{"tuya:kitchen": false},
		Sources: []SourceStatus{
			{Name: "tuya", LastOK: epoch.Add(-time.Minute)},
			{Name: "aqara", LastOK: epoch.Add(-40 * time.Minute), Stale: true},
			{Name: "weather", LastOK: epoch.Add(-12 * time.Minute)},
		},
		Extremes: map[model.DeviceID]map[model.Metric]Extremes{
			"tuya:office":  {model.Temperature: {Min: 20.1, Max: 22.4}},
			"tuya:bedroom": {model.Temperature: {Min: 19.8, Max: 22.0}},
			"tuya:hall":    {model.Temperature: {Min: 21.0, Max: 24.6}},
		},
		Trends: map[model.DeviceID]map[model.Metric]float64{
			"tuya:office":  {model.Temperature: 0.8},
			"tuya:hall":    {model.Temperature: -0.5},
			"tuya:kitchen": {model.Temperature: 0.1},
		},
		Alerts: []Alert{{Severity: SeverityUrgent, Message: "Протечка стиралки", Since: epoch.Add(-3 * time.Minute)}},
		Events: []model.Event{
			{Device: "aqara:lock", Kind: model.EventUnlocked, At: epoch.Add(-3 * time.Minute)},
			{Device: "tuya:kitchen", Kind: model.EventOffline, At: epoch.Add(-25 * time.Minute)},
		},
	}
	v.Seen = map[model.DeviceID]time.Time{}
	for id, byMetric := range readings {
		v.Readings[id] = map[model.Metric]model.Reading{}
		v.Seen[id] = epoch
		for m, val := range byMetric {
			v.Readings[id][m] = model.Reading{Device: id, Metric: m, Value: val, At: epoch}
		}
	}
	// The hall sensor has been silent since yesterday: stale on a healthy source.
	v.Seen["tuya:hall"] = epoch.Add(-26 * time.Hour)
	v.Stale["tuya:hall"] = true
	for _, id := range []model.DeviceID{"aqara:lock", "aqara:washer", "aqara:leak1", "aqara:leak2"} {
		v.Stale[id] = true
	}
	v.Stale["aqara:leak1"] = false // the alarming one stays visible as an alarm
	return v
}
