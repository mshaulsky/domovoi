package state

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

// State is the single owner of the current picture of the home.
type State struct {
	mu         sync.RWMutex
	devices    map[model.DeviceID]model.Device
	readings   map[model.DeviceID]map[model.Metric]model.Reading
	health     map[string]Health
	staleAfter map[string]time.Duration
}

// Health is the record of a source's last outcomes.
type Health struct {
	LastOK    time.Time // zero until the first successful batch
	LastError time.Time
	Err       string // "" while healthy
}

// Change is one reading that differs from what the state held. Old is nil
// when the metric was not known before.
type Change struct {
	Device model.DeviceID
	Metric model.Metric
	Old    *model.Reading
	New    model.Reading
}

// Snapshot is an immutable copy of the state at a moment.
type Snapshot struct {
	At         time.Time
	Devices    []model.Device // sorted by room, then name
	Readings   map[model.DeviceID]map[model.Metric]model.Reading
	Health     map[string]Health
	staleAfter map[string]time.Duration
}

// Staleness limits.
const (
	// DefaultStaleAfter applies to sources nobody configured.
	DefaultStaleAfter = 15 * time.Minute
	// DeviceStaleAfter is how long a device may stay silent before it counts
	// as stale even though its source is healthy: a cloud keeps echoing the
	// last values of a sensor that stopped reporting.
	DeviceStaleAfter = 2 * time.Hour
)

// New returns an empty state.
func New() *State {
	return &State{
		devices:    map[model.DeviceID]model.Device{},
		readings:   map[model.DeviceID]map[model.Metric]model.Reading{},
		health:     map[string]Health{},
		staleAfter: map[string]time.Duration{},
	}
}

// SetStaleAfter declares a configured source: how long it may go without a
// successful batch before its devices count as stale. The source gets a
// health record at once, so it shows in the header before its first answer.
func (s *State) SetStaleAfter(source string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staleAfter[source] = d
	if _, ok := s.health[source]; !ok {
		s.health[source] = Health{}
	}
}

// ApplyDevices upserts devices. Devices are never removed here: a vendor
// listing that omits one for a poll must not erase a device history refers
// to; hiding is a settings matter.
func (s *State) ApplyDevices(devices []model.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDevices(devices)
}

// ApplyReadings merges readings and reports the ones whose value changed.
// A reading older than the one held is ignored; an equal value only
// refreshes the observation time.
func (s *State) ApplyReadings(readings []model.Reading) []Change {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyReadings(readings)
}

// SetHealth records the outcome of a source's latest pass.
func (s *State) SetHealth(source string, err error, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setHealth(source, err, at)
}

// Apply records one source's successful pass atomically — its devices, its
// readings and a healthy mark at `at` — under a single lock, so a snapshot
// never sees a device without the readings that came with it. It reports
// the changes like ApplyReadings.
func (s *State) Apply(source string, devices []model.Device, readings []model.Reading, at time.Time) []Change {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDevices(devices)
	changes := s.applyReadings(readings)
	s.setHealth(source, nil, at)
	return changes
}

func (s *State) applyDevices(devices []model.Device) {
	for _, d := range devices {
		s.devices[d.ID] = d
	}
}

func (s *State) applyReadings(readings []model.Reading) []Change {
	var changes []Change
	for _, r := range readings {
		byMetric, ok := s.readings[r.Device]
		if !ok {
			byMetric = map[model.Metric]model.Reading{}
			s.readings[r.Device] = byMetric
		}
		old, had := byMetric[r.Metric]
		if had && r.At.Before(old.At) {
			continue
		}
		byMetric[r.Metric] = r
		if had && old.Value.Equal(r.Value) {
			continue
		}
		c := Change{Device: r.Device, Metric: r.Metric, New: r}
		if had {
			prev := old
			c.Old = &prev
		}
		changes = append(changes, c)
	}
	return changes
}

func (s *State) setHealth(source string, err error, at time.Time) {
	h := s.health[source]
	if err == nil {
		h.LastOK = at
		h.Err = ""
	} else {
		h.LastError = at
		h.Err = err.Error()
	}
	s.health[source] = h
}

// Snapshot copies the state as of now.
func (s *State) Snapshot(now time.Time) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn := Snapshot{
		At:         now,
		Devices:    make([]model.Device, 0, len(s.devices)),
		Readings:   make(map[model.DeviceID]map[model.Metric]model.Reading, len(s.readings)),
		Health:     make(map[string]Health, len(s.health)),
		staleAfter: make(map[string]time.Duration, len(s.staleAfter)),
	}
	sn.Devices = slices.AppendSeq(sn.Devices, maps.Values(s.devices))
	slices.SortFunc(sn.Devices, func(a, b model.Device) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Room), strings.ToLower(b.Room)),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.ID, b.ID),
		)
	})
	for id, byMetric := range s.readings {
		sn.Readings[id] = maps.Clone(byMetric)
	}
	maps.Copy(sn.Health, s.health)
	maps.Copy(sn.staleAfter, s.staleAfter)
	return sn
}

// Reading returns the latest reading of a metric.
func (sn Snapshot) Reading(id model.DeviceID, m model.Metric) (model.Reading, bool) {
	r, ok := sn.Readings[id][m]
	return r, ok
}

// Device returns a device by ID.
func (sn Snapshot) Device(id model.DeviceID) (model.Device, bool) {
	for _, d := range sn.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return model.Device{}, false
}

// Stale reports whether the device's data cannot be trusted as current: its
// source has not delivered recently, or the device itself has been silent
// for longer than DeviceStaleAfter.
func (sn Snapshot) Stale(id model.DeviceID) bool {
	if sn.SourceStale(id.Source()) {
		return true
	}
	seen, ok := sn.Seen(id)
	return ok && sn.At.Sub(seen) > DeviceStaleAfter
}

// Seen returns when the device last delivered any reading: the newest
// reading time it holds.
func (sn Snapshot) Seen(id model.DeviceID) (time.Time, bool) {
	var seen time.Time
	for _, r := range sn.Readings[id] {
		if r.At.After(seen) {
			seen = r.At
		}
	}
	return seen, !seen.IsZero()
}

// SourceStale reports whether a source has not delivered recently.
func (sn Snapshot) SourceStale(source string) bool {
	h, ok := sn.Health[source]
	if !ok || h.LastOK.IsZero() {
		return true
	}
	after, ok := sn.staleAfter[source]
	if !ok {
		after = DefaultStaleAfter
	}
	return sn.At.Sub(h.LastOK) > after
}

// Sources lists the sources with health records, sorted by name.
func (sn Snapshot) Sources() []string {
	return slices.Sorted(maps.Keys(sn.Health))
}
