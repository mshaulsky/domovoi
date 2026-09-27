package state

import (
	"errors"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
)

var (
	epoch   = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	bedroom = model.Device{ID: "tuya:bed", Name: "Спальня", Room: "Спальня", Kind: model.KindClimateSensor}
	hallLk  = model.Device{ID: "aqara:lock", Name: "Замок", Room: "Прихожая", Kind: model.KindLock}
	office  = model.Device{ID: "tuya:off", Name: "Кабинет", Room: "Кабинет", Kind: model.KindClimateSensor}
)

func reading(id model.DeviceID, m model.Metric, v model.Value, at time.Time) model.Reading {
	return model.Reading{Device: id, Metric: m, Value: v, At: at}
}

func TestNew(t *testing.T) {
	s := New()
	sn := s.Snapshot(epoch)
	if len(sn.Devices) != 0 || len(sn.Readings) != 0 || len(sn.Health) != 0 {
		t.Errorf("new state is not empty: %+v", sn)
	}
}

func TestStateSetStaleAfter(t *testing.T) {
	s := New()
	s.SetStaleAfter("tuya", time.Minute)
	if h, ok := s.Snapshot(epoch).Health["tuya"]; !ok || !h.LastOK.IsZero() {
		t.Errorf("a configured source should have an empty health record, got %+v, %t", h, ok)
	}
	s.SetHealth("tuya", nil, epoch)
	if s.Snapshot(epoch.Add(2*time.Minute)).SourceStale("tuya") != true {
		t.Error("source should be stale after its own stale-after")
	}
	if s.Snapshot(epoch.Add(30 * time.Second)).SourceStale("tuya") {
		t.Error("source should be fresh within its stale-after")
	}
}

func TestStateApplyDevices(t *testing.T) {
	s := New()
	s.ApplyDevices([]model.Device{bedroom, hallLk})
	renamed := bedroom
	renamed.Name = "Спальня 2"
	s.ApplyDevices([]model.Device{renamed})
	sn := s.Snapshot(epoch)
	if len(sn.Devices) != 2 {
		t.Fatalf("devices = %d, want 2 (upsert, never remove)", len(sn.Devices))
	}
	if d, _ := sn.Device("tuya:bed"); d.Name != "Спальня 2" {
		t.Errorf("device not updated: %+v", d)
	}
}

func TestStateApplyReadings(t *testing.T) {
	tests := []struct {
		name    string
		first   []model.Reading
		second  []model.Reading
		wantOld *model.Value // nil = new metric
		wantNew model.Value
		wantAt  time.Time
		wantN   int // changes reported by the second apply
	}{
		{
			name:    "first reading is a change without old",
			second:  []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch)},
			wantNew: model.NumberValue(21),
			wantAt:  epoch,
			wantN:   1,
		},
		{
			name:    "changed value reports old and new",
			first:   []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch)},
			second:  []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(22), epoch.Add(time.Minute))},
			wantOld: ptr(model.NumberValue(21)),
			wantNew: model.NumberValue(22),
			wantAt:  epoch.Add(time.Minute),
			wantN:   1,
		},
		{
			name:    "same value refreshes the time only",
			first:   []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch)},
			second:  []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch.Add(time.Minute))},
			wantNew: model.NumberValue(21),
			wantAt:  epoch.Add(time.Minute),
			wantN:   0,
		},
		{
			name:    "older reading ignored",
			first:   []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch)},
			second:  []model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(19), epoch.Add(-time.Minute))},
			wantNew: model.NumberValue(21),
			wantAt:  epoch,
			wantN:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			s.ApplyReadings(tt.first)
			changes := s.ApplyReadings(tt.second)
			if len(changes) != tt.wantN {
				t.Fatalf("changes = %d, want %d", len(changes), tt.wantN)
			}
			if tt.wantN == 1 {
				c := changes[0]
				if (c.Old == nil) != (tt.wantOld == nil) || (c.Old != nil && !c.Old.Value.Equal(*tt.wantOld)) {
					t.Errorf("change old = %v, want %v", c.Old, tt.wantOld)
				}
				if !c.New.Value.Equal(tt.wantNew) {
					t.Errorf("change new = %v, want %v", c.New.Value, tt.wantNew)
				}
			}
			r, ok := s.Snapshot(epoch).Reading("tuya:bed", model.Temperature)
			if !ok || !r.Value.Equal(tt.wantNew) || !r.At.Equal(tt.wantAt) {
				t.Errorf("held reading = %+v, ok=%t; want %v at %v", r, ok, tt.wantNew, tt.wantAt)
			}
		})
	}
}

func TestStateSetHealth(t *testing.T) {
	s := New()
	s.SetHealth("tuya", errors.New("boom"), epoch)
	h := s.Snapshot(epoch).Health["tuya"]
	if h.Err != "boom" || !h.LastError.Equal(epoch) || !h.LastOK.IsZero() {
		t.Errorf("after error: %+v", h)
	}
	s.SetHealth("tuya", nil, epoch.Add(time.Minute))
	h = s.Snapshot(epoch).Health["tuya"]
	if h.Err != "" || !h.LastOK.Equal(epoch.Add(time.Minute)) || !h.LastError.Equal(epoch) {
		t.Errorf("after recovery: %+v", h)
	}
}

func TestStateSnapshot(t *testing.T) {
	s := New()
	s.ApplyDevices([]model.Device{office, hallLk, bedroom})
	upper := model.Device{ID: "tuya:hall", Name: "Зал", Room: "Зал", Kind: model.KindClimateSensor}
	lower := model.Device{ID: "tuya:zed", Name: "зона", Room: "зона", Kind: model.KindClimateSensor}
	s.ApplyDevices([]model.Device{lower, upper})
	s.ApplyReadings([]model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(21), epoch)})
	sn := s.Snapshot(epoch)
	var got []model.DeviceID
	for _, d := range sn.Devices {
		got = append(got, d.ID)
	}
	want := []model.DeviceID{"tuya:hall", "tuya:zed", "tuya:off", "aqara:lock", "tuya:bed"} // by room then name, case-insensitively
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("device order = %v, want %v", got, want)
		}
	}
	// The snapshot is a copy: later applies must not show through.
	s.ApplyReadings([]model.Reading{reading("tuya:bed", model.Temperature, model.NumberValue(25), epoch.Add(time.Minute))})
	if r, _ := sn.Reading("tuya:bed", model.Temperature); r.Value.Num != 21 {
		t.Errorf("snapshot mutated: %v", r.Value)
	}
	if !sn.At.Equal(epoch) {
		t.Errorf("At = %v", sn.At)
	}
}

func TestSnapshotReading(t *testing.T) {
	s := New()
	s.ApplyReadings([]model.Reading{reading("tuya:bed", model.Humidity, model.NumberValue(48), epoch)})
	sn := s.Snapshot(epoch)
	if r, ok := sn.Reading("tuya:bed", model.Humidity); !ok || r.Value.Num != 48 {
		t.Errorf("Reading = %+v, %t", r, ok)
	}
	if _, ok := sn.Reading("tuya:bed", model.Temperature); ok {
		t.Error("unknown metric reported present")
	}
	if _, ok := sn.Reading("tuya:none", model.Humidity); ok {
		t.Error("unknown device reported present")
	}
}

func TestSnapshotDevice(t *testing.T) {
	s := New()
	s.ApplyDevices([]model.Device{bedroom})
	sn := s.Snapshot(epoch)
	if d, ok := sn.Device("tuya:bed"); !ok || d != bedroom {
		t.Errorf("Device = %+v, %t", d, ok)
	}
	if _, ok := sn.Device("tuya:none"); ok {
		t.Error("unknown device reported present")
	}
}

func TestSnapshotStale(t *testing.T) {
	tests := []struct {
		name       string
		lastOK     time.Time // zero = never
		staleAfter time.Duration
		seen       time.Time // zero = the device has no readings
		at         time.Time
		want       bool
	}{
		{name: "never polled", at: epoch, want: true},
		{name: "fresh", lastOK: epoch, staleAfter: 10 * time.Minute, at: epoch.Add(5 * time.Minute), want: false},
		{name: "just past", lastOK: epoch, staleAfter: 10 * time.Minute, at: epoch.Add(10*time.Minute + time.Second), want: true},
		{name: "default stale-after", lastOK: epoch, at: epoch.Add(DefaultStaleAfter + time.Second), want: true},
		{name: "default still fresh", lastOK: epoch, at: epoch.Add(DefaultStaleAfter - time.Second), want: false},
		{name: "silent device on a healthy source", lastOK: epoch, seen: epoch.Add(-DeviceStaleAfter - time.Second), at: epoch, want: true},
		{name: "recently heard device", lastOK: epoch, seen: epoch.Add(-DeviceStaleAfter + time.Minute), at: epoch, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			if tt.staleAfter > 0 {
				s.SetStaleAfter("tuya", tt.staleAfter)
			}
			if !tt.lastOK.IsZero() {
				s.SetHealth("tuya", nil, tt.lastOK)
			}
			if !tt.seen.IsZero() {
				s.ApplyReadings([]model.Reading{{Device: "tuya:bed", Metric: model.Temperature, Value: model.NumberValue(20), At: tt.seen}})
			}
			sn := s.Snapshot(tt.at)
			if got := sn.Stale("tuya:bed"); got != tt.want {
				t.Errorf("Stale = %t, want %t", got, tt.want)
			}
			if got := sn.SourceStale("tuya"); tt.seen.IsZero() && got != tt.want {
				t.Errorf("SourceStale = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSnapshotSeen(t *testing.T) {
	s := New()
	s.ApplyReadings([]model.Reading{
		{Device: "tuya:bed", Metric: model.Temperature, Value: model.NumberValue(20), At: epoch.Add(-time.Hour)},
		{Device: "tuya:bed", Metric: model.Humidity, Value: model.NumberValue(50), At: epoch},
	})
	sn := s.Snapshot(epoch)
	if seen, ok := sn.Seen("tuya:bed"); !ok || !seen.Equal(epoch) {
		t.Errorf("Seen = %v, %t; want the newest reading time", seen, ok)
	}
	if _, ok := sn.Seen("tuya:unknown"); ok {
		t.Error("a device without readings has no Seen time")
	}
}

func TestSnapshotSources(t *testing.T) {
	s := New()
	s.SetHealth("tuya", nil, epoch)
	s.SetHealth("aqara", nil, epoch)
	if got := s.Snapshot(epoch).Sources(); len(got) != 2 || got[0] != "aqara" || got[1] != "tuya" {
		t.Errorf("Sources() = %v", got)
	}
}

func TestClassify(t *testing.T) {
	old := func(v model.Value) *model.Reading {
		return &model.Reading{Device: "aqara:lock", Metric: model.Locked, Value: v, At: epoch}
	}
	tests := []struct {
		name    string
		changes []Change
		want    []model.EventKind
	}{
		{name: "none", changes: nil, want: nil},
		{
			name:    "first sighting is not a transition",
			changes: []Change{{Device: "aqara:lock", Metric: model.Locked, New: reading("aqara:lock", model.Locked, model.BoolValue(false), epoch)}},
			want:    nil,
		},
		{
			name:    "unlocked",
			changes: []Change{{Device: "aqara:lock", Metric: model.Locked, Old: old(model.BoolValue(true)), New: reading("aqara:lock", model.Locked, model.BoolValue(false), epoch)}},
			want:    []model.EventKind{model.EventUnlocked},
		},
		{
			name: "number change is not an event",
			changes: []Change{{Device: "tuya:bed", Metric: model.Temperature,
				Old: &model.Reading{Value: model.NumberValue(1)}, New: reading("tuya:bed", model.Temperature, model.NumberValue(2), epoch)}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := Classify(tt.changes)
			if len(events) != len(tt.want) {
				t.Fatalf("Classify = %v, want kinds %v", events, tt.want)
			}
			for i, e := range events {
				if e.Kind != tt.want[i] || e.Device != tt.changes[i].Device || !e.At.Equal(tt.changes[i].New.At) {
					t.Errorf("event %d = %+v, want kind %q", i, e, tt.want[i])
				}
			}
		})
	}
}

func ptr(v model.Value) *model.Value {
	return &v
}

func TestStateApply(t *testing.T) {
	s := New()
	dev := model.Device{ID: "tuya:a", Name: "A"}
	reading := model.Reading{Device: "tuya:a", Metric: model.Temperature, Value: model.NumberValue(21), At: epoch}
	changes := s.Apply("tuya", []model.Device{dev}, []model.Reading{reading}, epoch)
	if len(changes) != 1 || changes[0].Old != nil || changes[0].New != reading {
		t.Errorf("changes = %+v", changes)
	}
	sn := s.Snapshot(epoch)
	if _, ok := sn.Device(dev.ID); !ok {
		t.Error("device not applied")
	}
	if r, ok := sn.Reading(dev.ID, model.Temperature); !ok || r != reading {
		t.Errorf("reading = %+v, %t", r, ok)
	}
	if h := sn.Health["tuya"]; !h.LastOK.Equal(epoch) || h.Err != "" {
		t.Errorf("health = %+v", h)
	}
}
