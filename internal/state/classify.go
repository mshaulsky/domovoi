package state

import "github.com/mshaulsky/domovoi/internal/model"

// Classify turns changes into events using the transition table of the
// metric catalogue. A metric first seen is not a transition and produces
// nothing.
func Classify(changes []Change) []model.Event {
	var events []model.Event
	for _, c := range changes {
		if c.Old == nil {
			continue
		}
		kind, ok := c.Metric.Transition(c.Old.Value, c.New.Value)
		if !ok {
			continue
		}
		events = append(events, model.Event{Device: c.Device, Kind: kind, At: c.New.At})
	}
	return events
}
