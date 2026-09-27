package model

import "slices"

// WeatherCondition groups the WMO weather interpretation codes that
// Open-Meteo reports into the handful of states a dashboard can show.
type WeatherCondition string

// Weather conditions, coarse on purpose: each has an icon and a word.
const (
	WeatherClear        WeatherCondition = "clear"
	WeatherMainlyClear  WeatherCondition = "mainly_clear"
	WeatherPartlyCloudy WeatherCondition = "partly_cloudy"
	WeatherOvercast     WeatherCondition = "overcast"
	WeatherFog          WeatherCondition = "fog"
	WeatherDrizzle      WeatherCondition = "drizzle"
	WeatherRain         WeatherCondition = "rain"
	WeatherFreezingRain WeatherCondition = "freezing_rain"
	WeatherSnow         WeatherCondition = "snow"
	WeatherShowers      WeatherCondition = "showers"
	WeatherSnowShowers  WeatherCondition = "snow_showers"
	WeatherThunderstorm WeatherCondition = "thunderstorm"
	WeatherUnknown      WeatherCondition = "unknown"
)

// weatherConditions lists every condition, in declaration order.
var weatherConditions = []WeatherCondition{
	WeatherClear, WeatherMainlyClear, WeatherPartlyCloudy, WeatherOvercast, WeatherFog, WeatherDrizzle, WeatherRain,
	WeatherFreezingRain, WeatherSnow, WeatherShowers, WeatherSnowShowers, WeatherThunderstorm, WeatherUnknown,
}

// WeatherConditions lists every condition, for whatever must cover them
// all: the message catalogues, the icon table.
func WeatherConditions() []WeatherCondition {
	return slices.Clone(weatherConditions)
}

// WeatherConditionOf maps a WMO code (0–99) to its condition.
func WeatherConditionOf(code int) WeatherCondition {
	switch {
	case code == 0:
		return WeatherClear
	case code == 1:
		return WeatherMainlyClear
	case code == 2:
		return WeatherPartlyCloudy
	case code == 3:
		return WeatherOvercast
	case code == 45 || code == 48:
		return WeatherFog
	case code >= 51 && code <= 55:
		return WeatherDrizzle
	case code == 56 || code == 57 || code == 66 || code == 67:
		return WeatherFreezingRain
	case code >= 61 && code <= 65:
		return WeatherRain
	case code >= 71 && code <= 77:
		return WeatherSnow
	case code >= 80 && code <= 82:
		return WeatherShowers
	case code == 85 || code == 86:
		return WeatherSnowShowers
	case code >= 95 && code <= 99:
		return WeatherThunderstorm
	default:
		return WeatherUnknown
	}
}
