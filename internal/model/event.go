package model

import (
	"slices"
	"time"
)

// Event is a transition worth remembering, as opposed to a reading, which is a
// level: the door was unlocked, a leak started, a device went offline, the
// button was pressed, an alert was raised.
type Event struct {
	Device DeviceID  // empty for events that belong to no device
	Kind   EventKind // what happened
	Detail string    // "double" for a button press, the rule name for an alert
	At     time.Time
}

// EventKind names a transition.
type EventKind string

// Event kinds. The metric catalogue maps boolean transitions onto the first
// group; the rest are produced by watchers and by the alert engine.
const (
	EventUnlocked      EventKind = "unlocked"
	EventLocked        EventKind = "locked"
	EventLeakStarted   EventKind = "leak_started"
	EventLeakEnded     EventKind = "leak_ended"
	EventPoweredOn     EventKind = "powered_on"
	EventPoweredOff    EventKind = "powered_off"
	EventOffline       EventKind = "offline"
	EventOnline        EventKind = "online"
	EventButtonPressed EventKind = "button_pressed"
	EventAlertRaised   EventKind = "alert_raised"
	EventAlertResolved EventKind = "alert_resolved"
	EventAlertAcked    EventKind = "alert_acknowledged"
)

// eventKinds lists every kind, in declaration order.
var eventKinds = []EventKind{
	EventUnlocked, EventLocked, EventLeakStarted, EventLeakEnded, EventPoweredOn, EventPoweredOff,
	EventOffline, EventOnline, EventButtonPressed, EventAlertRaised, EventAlertResolved, EventAlertAcked,
}

// EventKinds lists every event kind, for whatever must cover them all: the
// message catalogues, the back-office filters.
func EventKinds() []EventKind {
	return slices.Clone(eventKinds)
}
