package model

import (
	"strconv"
	"strings"
	"time"
)

// Device is a physical or virtual device as a source reports it.
type Device struct {
	ID   DeviceID // its prefix names the source
	Name string   // as named in the vendor app
	Room string   // vendor room, empty when the vendor has none
	Kind Kind     // a hint for icons and rule targeting, not what scenes dispatch on
}

// Value is the value of a reading. Exactly one representation is meaningful,
// selected by Kind: Number and Bool use Num (Bool as 0 or 1), Text uses Text.
type Value struct {
	Kind ValueKind
	Num  float64
	Text string
}

// Reading is one observation of one metric of one device. The unit comes from
// the metric catalogue, not from the reading.
type Reading struct {
	Device DeviceID
	Metric Metric
	Value  Value
	At     time.Time // when the source observed it; the poll time if unknown
}

// DeviceID identifies a device across sources as "<source name>:<native id>",
// e.g. "tuya:bf22826zxhnglohk". The prefix is the name of the configured
// source section, which keeps native IDs unique within the home and must
// never change once history refers to it.
type DeviceID string

// Kind is a coarse device class.
type Kind string

// Metric names one reported quantity in vendor-neutral terms.
type Metric string

// ValueKind is the type tag of a Value.
type ValueKind int

// Device kinds.
const (
	KindClimateSensor Kind = "climate-sensor"
	KindLeakSensor    Kind = "leak-sensor"
	KindLock          Kind = "lock"
	KindOutlet        Kind = "outlet"
	KindButton        Kind = "button"
	KindThermostat    Kind = "thermostat"
	KindHub           Kind = "hub"
	KindVirtual       Kind = "virtual"
	KindOther         Kind = "other"
)

// Value kinds.
const (
	Number ValueKind = iota
	Bool
	Text
)

// NewDeviceID composes a device ID from a source name and a native ID.
func NewDeviceID(source, native string) DeviceID {
	return DeviceID(source + ":" + native)
}

// NumberValue makes a Number value.
func NumberValue(f float64) Value {
	return Value{Kind: Number, Num: f}
}

// BoolValue makes a Bool value.
func BoolValue(b bool) Value {
	v := Value{Kind: Bool}
	if b {
		v.Num = 1
	}
	return v
}

// TextValue makes a Text value.
func TextValue(s string) Value {
	return Value{Kind: Text, Text: s}
}

// Source returns the source prefix of the ID, or "" when it has none.
func (id DeviceID) Source() string {
	source, _, ok := strings.Cut(string(id), ":")
	if !ok {
		return ""
	}
	return source
}

// Bool reports a Bool value; for a Number it is true when non-zero, for a
// Text it is false.
func (v Value) Bool() bool {
	return v.Kind != Text && v.Num != 0
}

// Equal reports whether two values are the same kind and content.
func (v Value) Equal(o Value) bool {
	if v.Kind != o.Kind {
		return false
	}
	if v.Kind == Text {
		return v.Text == o.Text
	}
	return v.Num == o.Num
}

// String formats the value for logs: numbers shortest, bools as true/false.
func (v Value) String() string {
	switch v.Kind {
	case Bool:
		return strconv.FormatBool(v.Bool())
	case Text:
		return v.Text
	default:
		return strconv.FormatFloat(v.Num, 'f', -1, 64)
	}
}

// String names the value kind, for logs and errors.
func (k ValueKind) String() string {
	switch k {
	case Number:
		return "number"
	case Bool:
		return "bool"
	case Text:
		return "text"
	default:
		return "kind(" + strconv.Itoa(int(k)) + ")"
	}
}
