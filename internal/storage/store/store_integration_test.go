//go:build integration

package store

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/storage/devices"
	"github.com/mshaulsky/domovoi/internal/storage/events"
	"github.com/mshaulsky/domovoi/internal/storage/readings"
	"github.com/mshaulsky/domovoi/internal/storage/sqlite"
)

var (
	epoch = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
	hall  = model.Device{ID: "tuya:hall", Name: "Зал", Kind: model.KindClimateSensor}
	lock  = model.Device{ID: "aqara:lock", Name: "Door", Room: "Hallway", Kind: model.KindLock}
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "domovoi.db"), quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func num(id model.DeviceID, m model.Metric, v float64, at time.Time) model.Reading {
	return model.Reading{Device: id, Metric: m, Value: model.NumberValue(v), At: at}
}

func TestStorePersist(t *testing.T) {
	db := openDB(t)
	s := New(db)
	evs := []model.Event{{Device: lock.ID, Kind: model.EventUnlocked, Detail: "front", At: epoch}}
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 21.5, epoch),
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(false), At: epoch},
		{Device: hall.ID, Metric: model.TemperatureAlarm, Value: model.TextValue("cancel"), At: epoch},
	}
	if err := s.Persist(t.Context(), []model.Device{hall, lock}, rs, evs, epoch); err != nil {
		t.Fatal(err)
	}
	// A second pass updates the device and replaces the row at the same time.
	renamed := hall
	renamed.Name = "Living room"
	if err := s.Persist(t.Context(), []model.Device{renamed}, []model.Reading{num(hall.ID, model.Temperature, 21.7, epoch)}, nil, epoch.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	devs, err := devices.New(db).All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 || devs[1].Name != "Living room" || devs[0] != lock {
		t.Errorf("devices = %+v", devs)
	}
	last, ok, err := readings.New(db).Last(t.Context(), hall.ID, model.Temperature)
	if err != nil || !ok || last.Value.Num != 21.7 || !last.At.Equal(epoch) {
		t.Errorf("last = %+v, %t, %v", last, ok, err)
	}
	got, err := events.New(db).Since(t.Context(), epoch.Add(-time.Hour), 10)
	if err != nil || len(got) != 1 || got[0].Kind != evs[0].Kind || got[0].Device != evs[0].Device || got[0].Detail != evs[0].Detail || !got[0].At.Equal(evs[0].At) {
		t.Errorf("events = %+v, %v", got, err)
	}
}

func TestStoreRestore(t *testing.T) {
	db := openDB(t)
	s := New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-time.Hour)),
		num(hall.ID, model.Temperature, 22, epoch),
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch.Add(-time.Minute)},
	}
	if err := s.Persist(t.Context(), []model.Device{hall, lock}, rs, nil, epoch); err != nil {
		t.Fatal(err)
	}
	devs, got, err := s.Restore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Reading{
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch.Add(-time.Minute)},
		num(hall.ID, model.Temperature, 22, epoch),
	}
	if len(devs) != 2 || len(got) != 2 {
		t.Fatalf("restore = %d devices, %d readings", len(devs), len(got))
	}
	for i := range want {
		if got[i].Device != want[i].Device || got[i].Metric != want[i].Metric || !got[i].Value.Equal(want[i].Value) || !got[i].At.Equal(want[i].At) {
			t.Errorf("reading %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestStorePrune(t *testing.T) {
	db := openDB(t)
	s := New(db)
	// A device last seen two days ago with nothing newer: retired, its history aged out.
	ghost := model.Device{ID: "aqara:ghost", Name: "Old lock", Kind: model.KindLock}
	if err := s.Persist(t.Context(), []model.Device{ghost}, []model.Reading{num(ghost.ID, model.Battery, 90, epoch.Add(-48*time.Hour))}, nil, epoch.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rs := []model.Reading{num(hall.ID, model.Temperature, 20, epoch.Add(-48*time.Hour)), num(hall.ID, model.Temperature, 21, epoch)}
	evs := []model.Event{{Kind: model.EventOnline, Device: hall.ID, At: epoch.Add(-48 * time.Hour)}, {Kind: model.EventOffline, Device: hall.ID, At: epoch}}
	if err := s.Persist(t.Context(), []model.Device{hall}, rs, evs, epoch); err != nil {
		t.Fatal(err)
	}
	n, err := s.Prune(t.Context(), epoch.Add(-24*time.Hour))
	if err != nil || n != 4 { // two readings, one event, one device
		t.Fatalf("Prune = %d, %v; want 4", n, err)
	}
	if pts, err := readings.New(db).Series(t.Context(), hall.ID, model.Temperature, epoch.Add(-72*time.Hour)); err != nil || len(pts) != 1 {
		t.Errorf("series after prune = %+v, %v", pts, err)
	}
	if devs, _, err := s.Restore(t.Context()); err != nil || len(devs) != 1 || devs[0] != hall {
		t.Errorf("devices after prune = %+v, %v; want hall only", devs, err)
	}
}

func TestStoreExtremes(t *testing.T) {
	db := openDB(t)
	s := New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 19.5, epoch.Add(-3*time.Hour)),
		num(hall.ID, model.Temperature, 23, epoch.Add(-2*time.Hour)),
		num(hall.ID, model.Temperature, 21, epoch),
		num(hall.ID, model.Temperature, 10, epoch.Add(-30*time.Hour)), // yesterday: outside the window
	}
	if err := s.Persist(t.Context(), []model.Device{hall}, rs, nil, epoch); err != nil {
		t.Fatal(err)
	}
	lo, hi, ok, err := s.Extremes(t.Context(), hall.ID, model.Temperature, epoch.Add(-12*time.Hour))
	if err != nil || !ok || lo != 19.5 || hi != 23 {
		t.Errorf("Extremes = %v %v %t %v", lo, hi, ok, err)
	}
	if _, _, ok, err := s.Extremes(t.Context(), hall.ID, model.Humidity, epoch); ok || err != nil {
		t.Errorf("no series: ok=%t err=%v", ok, err)
	}
}

func TestStoreTrend(t *testing.T) {
	db := openDB(t)
	s := New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-90*time.Minute)),
		num(hall.ID, model.Temperature, 21.5, epoch),
	}
	if err := s.Persist(t.Context(), []model.Device{hall}, rs, nil, epoch); err != nil {
		t.Fatal(err)
	}
	delta, ok, err := s.Trend(t.Context(), hall.ID, model.Temperature, epoch.Add(-time.Hour))
	if err != nil || !ok || delta != 1.5 {
		t.Errorf("Trend = %v %t %v; want 1.5", delta, ok, err)
	}
	if _, ok, err := s.Trend(t.Context(), hall.ID, model.Temperature, epoch.Add(-2*time.Hour)); ok || err != nil {
		t.Errorf("no point before the window: ok=%t err=%v", ok, err)
	}
	if delta, ok, err := s.Trend(t.Context(), hall.ID, model.Temperature, epoch.Add(time.Hour)); !ok || delta != 0 || err != nil {
		t.Errorf("nothing newer than the window: delta=%v ok=%t err=%v; want flat", delta, ok, err)
	}
}

func TestStoreEvents(t *testing.T) {
	db := openDB(t)
	s := New(db)
	evs := []model.Event{
		{Kind: model.EventOnline, Device: hall.ID, At: epoch.Add(-2 * time.Hour)},
		{Kind: model.EventUnlocked, Device: lock.ID, At: epoch.Add(-time.Hour)},
		{Kind: model.EventLocked, Device: lock.ID, At: epoch},
	}
	if err := s.Persist(t.Context(), []model.Device{hall, lock}, nil, evs, epoch); err != nil {
		t.Fatal(err)
	}
	got, err := s.Events(t.Context(), epoch.Add(-90*time.Minute), 1)
	if err != nil || len(got) != 1 || got[0].Kind != model.EventLocked {
		t.Errorf("Events = %+v, %v; want the newest one", got, err)
	}
}
