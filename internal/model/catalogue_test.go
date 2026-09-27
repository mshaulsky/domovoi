package model

import "testing"

func TestLookup(t *testing.T) {
	tests := []struct {
		name   string
		metric Metric
		want   MetricInfo
		wantOK bool
	}{
		{name: "temperature", metric: Temperature, want: MetricInfo{Kind: Number, Unit: "°C"}, wantOK: true},
		{name: "leak", metric: Leak, want: MetricInfo{Kind: Bool, OnRise: EventLeakStarted, OnFall: EventLeakEnded}, wantOK: true},
		{name: "unknown", metric: "foo", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.metric)
			if ok != tt.wantOK {
				t.Fatalf("Lookup ok = %t, want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("Lookup = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMetricUnit(t *testing.T) {
	tests := []struct {
		name   string
		metric Metric
		want   string
	}{
		{name: "temperature", metric: Temperature, want: "°C"},
		{name: "humidity", metric: Humidity, want: "%"},
		{name: "bool has no unit", metric: Leak, want: ""},
		{name: "unknown", metric: "foo", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metric.Unit(); got != tt.want {
				t.Errorf("Unit() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMetricTransition(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metric
		old, now Value
		want     EventKind
		wantOK   bool
	}{
		{name: "unlocked", metric: Locked, old: BoolValue(true), now: BoolValue(false), want: EventUnlocked, wantOK: true},
		{name: "locked", metric: Locked, old: BoolValue(false), now: BoolValue(true), want: EventLocked, wantOK: true},
		{name: "leak started", metric: Leak, old: BoolValue(false), now: BoolValue(true), want: EventLeakStarted, wantOK: true},
		{name: "went offline", metric: Online, old: BoolValue(true), now: BoolValue(false), want: EventOffline, wantOK: true},
		{name: "no change", metric: Leak, old: BoolValue(true), now: BoolValue(true), wantOK: false},
		{name: "number metric", metric: Temperature, old: NumberValue(1), now: NumberValue(2), wantOK: false},
		{name: "wrong value kind", metric: Leak, old: NumberValue(0), now: NumberValue(1), wantOK: false},
		{name: "unknown metric", metric: "foo", old: BoolValue(false), now: BoolValue(true), wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.metric.Transition(tt.old, tt.now)
			if ok != tt.wantOK {
				t.Fatalf("Transition ok = %t, want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("Transition = %q, want %q", got, tt.want)
			}
		})
	}
}
