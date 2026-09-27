package model

import "testing"

func TestNewDeviceID(t *testing.T) {
	if got, want := NewDeviceID("tuya", "abc"), DeviceID("tuya:abc"); got != want {
		t.Errorf("NewDeviceID = %q, want %q", got, want)
	}
}

func TestDeviceIDSource(t *testing.T) {
	tests := []struct {
		name string
		id   DeviceID
		want string
	}{
		{name: "prefixed", id: "aqara:Aqr~1:2", want: "aqara"},
		{name: "bare", id: "abc", want: ""},
		{name: "empty", id: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Source(); got != tt.want {
				t.Errorf("Source() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueBool(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want bool
	}{
		{name: "bool true", v: BoolValue(true), want: true},
		{name: "bool false", v: BoolValue(false), want: false},
		{name: "number non-zero", v: NumberValue(22.5), want: true},
		{name: "number zero", v: NumberValue(0), want: false},
		{name: "text", v: TextValue("true"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Bool(); got != tt.want {
				t.Errorf("Bool() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestValueEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{name: "same number", a: NumberValue(1.5), b: NumberValue(1.5), want: true},
		{name: "different number", a: NumberValue(1.5), b: NumberValue(1.6), want: false},
		{name: "same text", a: TextValue("x"), b: TextValue("x"), want: true},
		{name: "different text", a: TextValue("x"), b: TextValue("y"), want: false},
		{name: "bool vs number", a: BoolValue(true), b: NumberValue(1), want: false},
		{name: "text ignores num", a: Value{Kind: Text, Num: 1}, b: Value{Kind: Text, Num: 2}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want string
	}{
		{name: "integer", v: NumberValue(22), want: "22"},
		{name: "fraction", v: NumberValue(22.4), want: "22.4"},
		{name: "bool", v: BoolValue(true), want: "true"},
		{name: "text", v: TextValue("upperalarm"), want: "upperalarm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueKindString(t *testing.T) {
	tests := []struct {
		name string
		k    ValueKind
		want string
	}{
		{name: "number", k: Number, want: "number"},
		{name: "bool", k: Bool, want: "bool"},
		{name: "text", k: Text, want: "text"},
		{name: "unknown", k: ValueKind(9), want: "kind(9)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.k.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
