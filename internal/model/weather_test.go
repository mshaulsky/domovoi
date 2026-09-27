package model

import (
	"slices"
	"testing"
)

func TestWeatherConditionOf(t *testing.T) {
	tests := []struct {
		name string
		code int
		want WeatherCondition
	}{
		{name: "clear", code: 0, want: WeatherClear},
		{name: "mainly clear", code: 1, want: WeatherMainlyClear},
		{name: "partly cloudy", code: 2, want: WeatherPartlyCloudy},
		{name: "overcast", code: 3, want: WeatherOvercast},
		{name: "fog", code: 45, want: WeatherFog},
		{name: "rime fog", code: 48, want: WeatherFog},
		{name: "drizzle", code: 53, want: WeatherDrizzle},
		{name: "freezing drizzle", code: 56, want: WeatherFreezingRain},
		{name: "rain", code: 63, want: WeatherRain},
		{name: "freezing rain", code: 67, want: WeatherFreezingRain},
		{name: "snow", code: 73, want: WeatherSnow},
		{name: "snow grains", code: 77, want: WeatherSnow},
		{name: "showers", code: 81, want: WeatherShowers},
		{name: "snow showers", code: 86, want: WeatherSnowShowers},
		{name: "thunderstorm", code: 95, want: WeatherThunderstorm},
		{name: "thunderstorm with hail", code: 99, want: WeatherThunderstorm},
		{name: "unassigned code", code: 30, want: WeatherUnknown},
		{name: "negative", code: -1, want: WeatherUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeatherConditionOf(tt.code); got != tt.want {
				t.Errorf("WeatherConditionOf(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestWeatherConditions(t *testing.T) {
	all := WeatherConditions()
	if len(all) != 13 || all[len(all)-1] != WeatherUnknown {
		t.Errorf("WeatherConditions() = %v", all)
	}
	for code := range 100 {
		if c := WeatherConditionOf(code); !slices.Contains(all, c) {
			t.Errorf("WMO %d maps to %q, which is not listed", code, c)
		}
	}
}
