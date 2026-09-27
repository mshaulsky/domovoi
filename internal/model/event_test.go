package model

import (
	"slices"
	"testing"
)

func TestEventKinds(t *testing.T) {
	kinds := EventKinds()
	if len(kinds) != 12 || !slices.Contains(kinds, EventUnlocked) || !slices.Contains(kinds, EventAlertAcked) {
		t.Errorf("EventKinds() = %v", kinds)
	}
	kinds[0] = "mutated"
	if EventKinds()[0] == "mutated" {
		t.Error("EventKinds() must return a copy")
	}
}
