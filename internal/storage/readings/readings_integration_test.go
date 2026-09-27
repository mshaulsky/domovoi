//go:build integration

package readings_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/storage/devices"
	"github.com/mshaulsky/domovoi/internal/storage/readings"
	"github.com/mshaulsky/domovoi/internal/storage/sqlite"
)

var (
	epoch = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	hall  = model.Device{ID: "tuya:hall", Name: "Зал", Kind: model.KindClimateSensor}
	lock  = model.Device{ID: "aqara:lock", Name: "Door", Kind: model.KindLock}
)

// openDB opens a fresh database with the two devices the series refer to.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "domovoi.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := devices.New(db).Upsert(t.Context(), []model.Device{hall, lock}, epoch); err != nil {
		t.Fatal(err)
	}
	return db
}

func num(id model.DeviceID, m model.Metric, v float64, at time.Time) model.Reading {
	return model.Reading{Device: id, Metric: m, Value: model.NumberValue(v), At: at}
}

// samePoint compares a point by time and value, ignoring the zone.
func samePoint(p readings.Point, at time.Time, v model.Value) bool {
	return p.At.Equal(at) && p.Value.Equal(v)
}

func TestRepositoryWrite(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 21.5, epoch),
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch},
		{Device: hall.ID, Metric: model.TemperatureAlarm, Value: model.TextValue("cancel"), At: epoch},
		num(hall.ID, model.Temperature, 21.7, epoch), // same series and time: replaces the row
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	var series, rows int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM series`).Scan(&series); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM readings`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if series != 3 || rows != 3 {
		t.Errorf("series = %d rows = %d, want 3 and 3", series, rows)
	}
	var unit string
	if err := db.QueryRowContext(t.Context(), `SELECT unit FROM series WHERE device = ? AND metric = ?`, string(hall.ID), string(model.Temperature)).Scan(&unit); err != nil || unit != "°C" {
		t.Errorf("unit = %q, %v", unit, err)
	}
	if err := r.Write(t.Context(), nil); err != nil {
		t.Errorf("empty write = %v", err)
	}
}

func TestRepositorySeries(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-2*time.Hour)),
		num(hall.ID, model.Temperature, 21, epoch.Add(-time.Hour)),
		num(hall.ID, model.Temperature, 22, epoch),
		num(hall.ID, model.Humidity, 50, epoch), // another series
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	pts, err := r.Series(t.Context(), hall.ID, model.Temperature, epoch.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 || !samePoint(pts[0], epoch.Add(-time.Hour), model.NumberValue(21)) || !samePoint(pts[1], epoch, model.NumberValue(22)) {
		t.Errorf("Series = %+v", pts)
	}
	if pts, err := r.Series(t.Context(), lock.ID, model.Locked, epoch.Add(-time.Hour)); err != nil || len(pts) != 0 {
		t.Errorf("unknown series = %+v, %v", pts, err)
	}
}

func TestRepositoryExtremes(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 10, epoch.Add(-30*time.Hour)), // outside the window
		num(hall.ID, model.Temperature, 19.5, epoch.Add(-3*time.Hour)),
		num(hall.ID, model.Temperature, 23, epoch.Add(-2*time.Hour)),
		num(hall.ID, model.Temperature, 21, epoch),
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	lo, hi, ok, err := r.Extremes(t.Context(), hall.ID, model.Temperature, epoch.Add(-12*time.Hour))
	if err != nil || !ok || !samePoint(lo, epoch.Add(-3*time.Hour), model.NumberValue(19.5)) || !samePoint(hi, epoch.Add(-2*time.Hour), model.NumberValue(23)) {
		t.Errorf("Extremes = %+v %+v %t %v", lo, hi, ok, err)
	}
	if _, _, ok, err := r.Extremes(t.Context(), hall.ID, model.Humidity, epoch); ok || err != nil {
		t.Errorf("no series: ok=%t err=%v", ok, err)
	}
}

func TestRepositoryLast(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(false), At: epoch.Add(-time.Hour)},
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch},
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	p, ok, err := r.Last(t.Context(), lock.ID, model.Locked)
	if err != nil || !ok || !samePoint(p, epoch, model.BoolValue(true)) || p.Value.Kind != model.Bool {
		t.Errorf("Last = %+v %t %v; want a Bool true at epoch", p, ok, err)
	}
	if _, ok, err := r.Last(t.Context(), hall.ID, model.Temperature); ok || err != nil {
		t.Errorf("no series: ok=%t err=%v", ok, err)
	}
}

func TestRepositoryBefore(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-2*time.Hour)),
		num(hall.ID, model.Temperature, 21, epoch.Add(-time.Hour)),
		num(hall.ID, model.Temperature, 22, epoch),
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		t      time.Time
		want   float64
		wantOK bool
	}{
		{name: "between points", t: epoch.Add(-90 * time.Minute), want: 20, wantOK: true},
		{name: "exactly on a point", t: epoch.Add(-time.Hour), want: 21, wantOK: true},
		{name: "before everything", t: epoch.Add(-3 * time.Hour), wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok, err := r.Before(t.Context(), hall.ID, model.Temperature, tt.t)
			if err != nil || ok != tt.wantOK || (ok && p.Value.Num != tt.want) {
				t.Errorf("Before = %+v %t %v; want %v %t", p, ok, err, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRepositoryLastAll(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-time.Hour)),
		num(hall.ID, model.Temperature, 22, epoch),
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch.Add(-time.Minute)},
		{Device: hall.ID, Metric: model.TemperatureAlarm, Value: model.TextValue("cancel"), At: epoch},
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	got, err := r.LastAll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Reading{
		{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(true), At: epoch.Add(-time.Minute)},
		num(hall.ID, model.Temperature, 22, epoch),
		{Device: hall.ID, Metric: model.TemperatureAlarm, Value: model.TextValue("cancel"), At: epoch},
	}
	if len(got) != len(want) {
		t.Fatalf("LastAll = %d readings, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Device != want[i].Device || got[i].Metric != want[i].Metric || !got[i].Value.Equal(want[i].Value) || !got[i].At.Equal(want[i].At) {
			t.Errorf("reading %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRepositoryPrune(t *testing.T) {
	db := openDB(t)
	r := readings.New(db)
	rs := []model.Reading{
		num(hall.ID, model.Temperature, 20, epoch.Add(-48*time.Hour)),
		num(hall.ID, model.Temperature, 21, epoch.Add(-36*time.Hour)),
		num(hall.ID, model.Temperature, 22, epoch),
		num(hall.ID, model.Humidity, 50, epoch.Add(-48*time.Hour)), // a series that empties
	}
	if err := r.Write(t.Context(), rs); err != nil {
		t.Fatal(err)
	}
	n, err := r.Prune(t.Context(), epoch.Add(-24*time.Hour))
	if err != nil || n != 3 {
		t.Fatalf("Prune = %d, %v; want 3", n, err)
	}
	if pts, err := r.Series(t.Context(), hall.ID, model.Temperature, epoch.Add(-72*time.Hour)); err != nil || len(pts) != 1 {
		t.Errorf("after prune = %+v, %v", pts, err)
	}
	var series int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM series WHERE device = ?`, string(hall.ID)).Scan(&series); err != nil || series != 1 {
		t.Errorf("series left = %d, %v; want 1 (the emptied one dropped)", series, err)
	}
}
