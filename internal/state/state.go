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
	observed   map[series]observation
	health     map[string]Health
	staleAfter map[string]time.Duration
	restored   bool // seeded from storage and not yet refreshed by a live batch
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
	Restored   bool // the picture comes from storage; no source has delivered since start
	staleAfter map[string]time.Duration
}

// Staleness limits.
const (
	// DefaultStaleAfter applies to sources nobody configured.
	DefaultStaleAfter = 15 * time.Minute
	// DeviceStaleAfter is how long a device may show no sign of life — no
	// value change, no network event — before it counts as stale even
	// though its source is healthy: a cloud keeps echoing the last values
	// of a sensor that stopped reporting. Three hours, because a quiet room
	// can hold the same reading for a while.
	DeviceStaleAfter = 3 * time.Hour
)

// series identifies one metric of one device.
type series struct {
	device model.DeviceID
	metric model.Metric
}

// observation is what the state remembers about a series besides its held
// reading: the last batch that carried it and the newest stamp its source
// has ever given it. A restored series has neither.
type observation struct {
	at     time.Time
	vendor time.Time
}

// New returns an empty state.
func New() *State {
	return &State{
		devices:    map[model.DeviceID]model.Device{},
		readings:   map[model.DeviceID]map[model.Metric]model.Reading{},
		observed:   map[series]observation{},
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

// ApplyReadings merges readings on their own stamps and reports the ones
// whose value changed. There is no batch moment, so nothing is re-stamped:
// sources go through Apply, this is for seeding a state by hand.
func (s *State) ApplyReadings(readings []model.Reading) []Change {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyReadings(readings, time.Time{})
}

// SetHealth records the outcome of a source's latest pass.
func (s *State) SetHealth(source string, err error, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setHealth(source, err, at)
}

// Apply records one source's successful pass atomically — its devices, its
// readings and a healthy mark at `at` — under a single lock, so a snapshot
// never sees a device without the readings that came with it. A device list
// is the source's whole inventory: its devices missing from it are retired,
// and their IDs are returned next to the changes, which are reported like
// ApplyReadings. An empty list is distrusted — a cloud answering nothing is
// far likelier than a home with nothing in it — and retires no one.
func (s *State) Apply(source string, devices []model.Device, readings []model.Reading, at time.Time) ([]Change, []model.DeviceID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDevices(devices)
	var retired []model.DeviceID
	if len(devices) > 0 {
		retired = s.retire(source, devices)
	}
	changes := s.applyReadings(readings, at)
	s.setHealth(source, nil, at)
	s.restored = false
	return changes, retired
}

// Restore seeds the state from storage after a restart: devices and the
// last known readings, applied without reporting changes (nothing
// happened, it is remembered) and marked as restored until a live batch
// replaces the picture.
func (s *State) Restore(devices []model.Device, readings []model.Reading) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyDevices(devices)
	for _, r := range readings {
		byMetric, ok := s.readings[r.Device]
		if !ok {
			byMetric = map[model.Metric]model.Reading{}
			s.readings[r.Device] = byMetric
		}
		byMetric[r.Metric] = r
	}
	s.restored = true
}

func (s *State) applyDevices(devices []model.Device) {
	for _, d := range devices {
		s.devices[d.ID] = d
	}
}

// lastSeen is the latest moment a series' held value is known to have been
// true: its last observation, or its own stamp when it was restored.
func lastSeen(obs observation, held model.Reading) time.Time {
	if obs.at.After(held.At) {
		return obs.at
	}
	return held.At
}

// retire drops the source's devices that its listing no longer names,
// readings included — a device removed from the account, re-paired under a
// new ID, or seen through another account — and returns their IDs, sorted.
// Their history stays in storage.
func (s *State) retire(source string, listed []model.Device) []model.DeviceID {
	keep := make(map[model.DeviceID]bool, len(listed))
	for _, d := range listed {
		keep[d.ID] = true
	}
	gone := map[model.DeviceID]bool{}
	for id := range s.devices {
		if id.Source() == source && !keep[id] {
			gone[id] = true
		}
	}
	for id := range s.readings {
		if id.Source() == source && !keep[id] {
			gone[id] = true
		}
	}
	retired := make([]model.DeviceID, 0, len(gone))
	for id := range gone {
		delete(s.devices, id)
		for metric := range s.readings[id] {
			delete(s.observed, series{id, metric})
		}
		delete(s.readings, id)
		retired = append(retired, id)
	}
	slices.Sort(retired)
	return retired
}

// applyReadings merges readings carried by a batch at `at` (zero: no batch)
// and reports the changes. A reading older than the newest stamp its source
// ever gave the series is stale and ignored. The held value seen again only
// moves the observation, and its stamp if the source's is newer — a network
// event is a sign of life. A changed value keeps its stamp only when that is
// newer than the last moment the previous value was known to hold (its last
// observation, or its own stamp when restored); otherwise it takes the
// batch time. Vendors like Tuya date a device by its last network event,
// not its last report, so a stamp may stand still across several changes
// or point into a past in which the old value was still being observed —
// and the heartbeat row written at that observation must stay behind the
// change.
func (s *State) applyReadings(readings []model.Reading, at time.Time) []Change {
	var changes []Change
	for _, r := range readings {
		byMetric, ok := s.readings[r.Device]
		if !ok {
			byMetric = map[model.Metric]model.Reading{}
			s.readings[r.Device] = byMetric
		}
		key := series{r.Device, r.Metric}
		obs := s.observed[key]
		old, had := byMetric[r.Metric]
		if had && r.At.Before(obs.vendor) {
			continue
		}
		seen := lastSeen(obs, old) // before this batch counts as one
		if r.At.After(obs.vendor) {
			obs.vendor = r.At
		}
		if !at.IsZero() {
			obs.at = at
		}
		s.observed[key] = obs
		if had && old.Value.Equal(r.Value) {
			if r.At.After(old.At) {
				byMetric[r.Metric] = r
			}
			continue
		}
		if had && !r.At.After(seen) && at.After(r.At) {
			r.At = at
		}
		byMetric[r.Metric] = r
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
		Restored:   s.restored,
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
