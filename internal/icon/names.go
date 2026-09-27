package icon

import (
	"maps"
	"slices"

	"github.com/mshaulsky/domovoi/internal/model"
)

// Icon is a code point of the icon font.
type Icon rune

// Icons used by the scenes. Values are the upstream code points; the subset
// font must contain each (see the package documentation).
const (
	None Icon = 0

	Thermometer    Icon = 0xF050F
	Humidity       Icon = 0xF058E // water-percent
	Battery        Icon = 0xF0079
	Battery10      Icon = 0xF007A
	Battery20      Icon = 0xF007B
	Battery30      Icon = 0xF007C
	Battery40      Icon = 0xF007D
	Battery50      Icon = 0xF007E
	Battery60      Icon = 0xF007F
	Battery70      Icon = 0xF0080
	Battery80      Icon = 0xF0081
	Battery90      Icon = 0xF0082
	BatteryEmpty   Icon = 0xF008E // battery-outline
	BatteryUnknown Icon = 0xF0091
	BatteryAlert   Icon = 0xF10CD // battery-alert-variant-outline

	Lock         Icon = 0xF033E
	LockOpen     Icon = 0xF0FC6 // lock-open-variant
	PowerPlug    Icon = 0xF06A5
	PowerPlugOff Icon = 0xF06A6
	Water        Icon = 0xF058C
	WaterOff     Icon = 0xF058D
	Washer       Icon = 0xF072A // washing-machine
	Thermostat   Icon = 0xF0393
	Hub          Icon = 0xF1C96 // hub-outline
	Button       Icon = 0xF12A8 // gesture-tap-button
	Lightbulb    Icon = 0xF0335
	Socket       Icon = 0xF07E7 // power-socket-eu
	Radiator     Icon = 0xF0438

	Home     Icon = 0xF02DC
	Bed      Icon = 0xF02E3
	Sofa     Icon = 0xF04B9
	Desk     Icon = 0xF1239
	Stove    Icon = 0xF04DE
	Location Icon = 0xF07D9 // map-marker-outline

	WeatherSunny          Icon = 0xF0599
	WeatherPartlyCloudy   Icon = 0xF0595
	WeatherCloudy         Icon = 0xF0590
	WeatherFog            Icon = 0xF0591
	WeatherRainy          Icon = 0xF0597
	WeatherPouring        Icon = 0xF0596
	WeatherPartlyRainy    Icon = 0xF0F33
	WeatherSnowy          Icon = 0xF0598
	WeatherSnowyHeavy     Icon = 0xF0F36
	WeatherSnowyRainy     Icon = 0xF067F
	WeatherHail           Icon = 0xF0592
	WeatherLightning      Icon = 0xF0593
	WeatherLightningRainy Icon = 0xF067E
	WeatherWindy          Icon = 0xF059D
	WeatherNight          Icon = 0xF0594
	Sunrise               Icon = 0xF059C // weather-sunset-up
	Sunset                Icon = 0xF059B // weather-sunset-down

	Alert    Icon = 0xF0026
	Bell     Icon = 0xF009A
	Info     Icon = 0xF02FD // information-outline
	Check    Icon = 0xF05E1 // check-circle-outline
	Help     Icon = 0xF0625 // help-circle-outline
	WifiOff  Icon = 0xF05AA
	CloudOff Icon = 0xF0164 // cloud-off-outline

	TrendUp      Icon = 0xF0535
	TrendDown    Icon = 0xF0533
	TrendFlat    Icon = 0xF0534 // trending-neutral
	ArrowUp      Icon = 0xF19B2 // arrow-up-thin
	ArrowDown    Icon = 0xF19B3
	ArrowRight   Icon = 0xF19B0
	History      Icon = 0xF02DA
	Clock        Icon = 0xF0150 // clock-outline
	Update       Icon = 0xF06B0
	Calendar     Icon = 0xF00F6 // calendar-today
	ClosedLocked Icon = 0xF10AF // door-closed-lock
)

// names is the whitelist a settings override may use ("icon: bed"), and the
// source of the code point list for regenerating the subset.
var names = map[string]Icon{
	"thermometer":             Thermometer,
	"humidity":                Humidity,
	"battery":                 Battery,
	"lock":                    Lock,
	"lock-open":               LockOpen,
	"power-plug":              PowerPlug,
	"power-plug-off":          PowerPlugOff,
	"water":                   Water,
	"water-off":               WaterOff,
	"washer":                  Washer,
	"thermostat":              Thermostat,
	"hub":                     Hub,
	"button":                  Button,
	"lightbulb":               Lightbulb,
	"socket":                  Socket,
	"radiator":                Radiator,
	"home":                    Home,
	"bed":                     Bed,
	"sofa":                    Sofa,
	"desk":                    Desk,
	"stove":                   Stove,
	"location":                Location,
	"alert":                   Alert,
	"bell":                    Bell,
	"info":                    Info,
	"check":                   Check,
	"help":                    Help,
	"wifi-off":                WifiOff,
	"cloud-off":               CloudOff,
	"history":                 History,
	"clock":                   Clock,
	"update":                  Update,
	"calendar":                Calendar,
	"door-locked":             ClosedLocked,
	"battery-10":              Battery10,
	"battery-20":              Battery20,
	"battery-30":              Battery30,
	"battery-40":              Battery40,
	"battery-50":              Battery50,
	"battery-60":              Battery60,
	"battery-70":              Battery70,
	"battery-80":              Battery80,
	"battery-90":              Battery90,
	"battery-empty":           BatteryEmpty,
	"battery-unknown":         BatteryUnknown,
	"battery-alert":           BatteryAlert,
	"trend-up":                TrendUp,
	"trend-down":              TrendDown,
	"trend-flat":              TrendFlat,
	"arrow-up":                ArrowUp,
	"arrow-down":              ArrowDown,
	"arrow-right":             ArrowRight,
	"sunrise":                 Sunrise,
	"sunset":                  Sunset,
	"weather-sunny":           WeatherSunny,
	"weather-cloudy":          WeatherCloudy,
	"weather-fog":             WeatherFog,
	"weather-rainy":           WeatherRainy,
	"weather-snowy":           WeatherSnowy,
	"weather-windy":           WeatherWindy,
	"weather-night":           WeatherNight,
	"weather-hail":            WeatherHail,
	"weather-pouring":         WeatherPouring,
	"weather-partly-cloudy":   WeatherPartlyCloudy,
	"weather-partly-rainy":    WeatherPartlyRainy,
	"weather-snowy-heavy":     WeatherSnowyHeavy,
	"weather-snowy-rainy":     WeatherSnowyRainy,
	"weather-lightning":       WeatherLightning,
	"weather-lightning-rainy": WeatherLightningRainy,
}

// conditions maps a weather condition to its icon.
var conditions = map[model.WeatherCondition]Icon{
	model.WeatherClear:        WeatherSunny,
	model.WeatherMainlyClear:  WeatherPartlyCloudy,
	model.WeatherPartlyCloudy: WeatherPartlyCloudy,
	model.WeatherOvercast:     WeatherCloudy,
	model.WeatherFog:          WeatherFog,
	model.WeatherDrizzle:      WeatherPartlyRainy,
	model.WeatherRain:         WeatherRainy,
	model.WeatherFreezingRain: WeatherSnowyRainy,
	model.WeatherSnow:         WeatherSnowy,
	model.WeatherShowers:      WeatherPouring,
	model.WeatherSnowShowers:  WeatherSnowyHeavy,
	model.WeatherThunderstorm: WeatherLightningRainy,
	model.WeatherUnknown:      Help,
}

// kinds maps a device kind to its default icon.
var kinds = map[model.Kind]Icon{
	model.KindClimateSensor: Thermometer,
	model.KindLeakSensor:    Water,
	model.KindLock:          Lock,
	model.KindOutlet:        PowerPlug,
	model.KindButton:        Button,
	model.KindThermostat:    Thermostat,
	model.KindHub:           Hub,
	model.KindVirtual:       Location,
	model.KindOther:         Help,
}

// batteryLevels are the battery icons from full to empty, each with the
// lowest percentage it represents.
var batteryLevels = []struct {
	min  int
	icon Icon
}{
	{95, Battery}, {85, Battery90}, {75, Battery80}, {65, Battery70}, {55, Battery60},
	{45, Battery50}, {35, Battery40}, {25, Battery30}, {15, Battery20}, {5, Battery10},
	{0, BatteryEmpty},
}

// Lookup resolves a settings-level icon name.
func Lookup(name string) (Icon, bool) {
	ic, ok := names[name]
	return ic, ok
}

// Names lists the icon names a settings override may use, sorted.
func Names() []string {
	return slices.Sorted(maps.Keys(names))
}

// ForCondition returns the icon of a weather condition.
func ForCondition(c model.WeatherCondition) Icon {
	if ic, ok := conditions[c]; ok {
		return ic
	}
	return Help
}

// ForKind returns the default icon of a device kind.
func ForKind(k model.Kind) Icon {
	if ic, ok := kinds[k]; ok {
		return ic
	}
	return Help
}

// ForBattery returns the icon for a battery percentage; out-of-range values
// clamp, and a negative one means unknown.
func ForBattery(percent int) Icon {
	if percent < 0 {
		return BatteryUnknown
	}
	for _, l := range batteryLevels {
		if percent >= l.min {
			return l.icon
		}
	}
	return BatteryEmpty
}

// ForTrend returns the arrow for a change: rising, falling or flat within
// the dead band.
func ForTrend(delta, deadBand float64) Icon {
	switch {
	case delta > deadBand:
		return TrendUp
	case delta < -deadBand:
		return TrendDown
	default:
		return TrendFlat
	}
}

// String renders the icon as the string a font.Drawer takes.
func (i Icon) String() string {
	return string(rune(i))
}
