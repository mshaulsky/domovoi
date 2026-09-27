package core

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/state"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type deps struct {
	ctrl    *gomock.Controller
	state   *state.State
	metrics *MockMetrics
	core    *Core
}

func newCore(t *testing.T) deps {
	t.Helper()
	ctrl := gomock.NewController(t)
	d := deps{ctrl: ctrl, state: state.New(), metrics: NewMockMetrics(ctrl)}
	d.core = New(d.state, Config{}, quiet, d.metrics)
	return d
}

func namedCoordinator(ctrl *gomock.Controller, name string) *MockCoordinator {
	co := NewMockCoordinator(ctrl)
	co.EXPECT().Name().Return(name).AnyTimes()
	return co
}

func TestNew(t *testing.T) {
	d := newCore(t)
	if d.core.state != d.state || len(d.core.pollers) != 0 || len(d.core.coordinators) != 0 {
		t.Errorf("core = %+v", d.core)
	}
	if d.core.cfg.Heartbeat != DefaultHeartbeat || d.core.cfg.Retention != DefaultRetention {
		t.Errorf("defaults not applied: %+v", d.core.cfg)
	}
}

func TestCoreAddPoller(t *testing.T) {
	d := newCore(t)
	d.core.AddPoller("tuya", NewMockPoller(d.ctrl))
	if _, ok := d.core.pollers["tuya"]; !ok || !d.core.pending["tuya"] {
		t.Error("poller not registered as pending")
	}
}

func TestCoreAddCoordinator(t *testing.T) {
	d := newCore(t)
	d.core.AddCoordinator(namedCoordinator(d.ctrl, "hall"))
	if _, ok := d.core.coordinators["hall"]; !ok {
		t.Error("coordinator not registered")
	}
}

func TestCoreBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		d := newCore(t)
		co := namedCoordinator(d.ctrl, "hall")
		d.core.AddCoordinator(co)
		d.core.AddPoller("aqara", NewMockPoller(d.ctrl))
		co.EXPECT().Wake().Times(1) // once, when the only pending source reports
		d.metrics.EXPECT().IncEvent("unlocked").Times(1)

		lock := model.Device{ID: "aqara:lock", Name: "Lock", Kind: model.KindLock}
		locked := func(v bool, at time.Time) source.Batch {
			return source.Batch{Devices: []model.Device{lock}, Readings: []model.Reading{{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(v), At: at}}}
		}
		d.core.Batch("aqara", locked(true, start))
		time.Sleep(time.Minute)
		d.core.Batch("aqara", locked(false, start.Add(time.Minute)))

		sn := d.state.Snapshot(time.Now())
		if _, ok := sn.Device(lock.ID); !ok {
			t.Error("device not applied")
		}
		if r, _ := sn.Reading(lock.ID, model.Locked); r.Value.Bool() {
			t.Error("reading not applied")
		}
		if h := sn.Health["aqara"]; !h.LastOK.Equal(start.Add(time.Minute)) || h.Err != "" {
			t.Errorf("health = %+v", h)
		}
	})
}

func TestCoreEvent(t *testing.T) {
	d := newCore(t)
	d.metrics.EXPECT().IncEvent("button_pressed")
	d.core.Event(model.Event{Device: "mqtt:button", Kind: model.EventButtonPressed, Detail: "double", At: time.Now()})
}

func TestCoreHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		d := newCore(t)
		co := namedCoordinator(d.ctrl, "hall")
		d.core.AddCoordinator(co)
		d.core.AddPoller("tuya", NewMockPoller(d.ctrl))
		d.core.AddPoller("aqara", NewMockPoller(d.ctrl))
		co.EXPECT().Wake().Times(2) // when the last source answers, and when tuya first delivers data

		d.core.Health("tuya", errors.New("down"))
		if h := d.state.Snapshot(start).Health["tuya"]; h.Err != "down" || !h.LastError.Equal(start) {
			t.Errorf("health = %+v", h)
		}
		if len(d.core.pending) != 1 {
			t.Error("a failing source should still count as reported")
		}
		d.core.Health("aqara", nil) // the last pending one: displays wake
		d.core.Health("aqara", nil) // reported again: no second wake
		d.core.Health("tuya", errors.New("still down"))
		d.core.Batch("tuya", source.Batch{}) // first data after a failed start: wake
		d.core.Batch("tuya", source.Batch{}) // routine data: no wake
	})
}

func TestCoreStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newCore(t)
		p := NewMockPoller(d.ctrl)
		co := namedCoordinator(d.ctrl, "hall")
		ran := make(chan string, 2)
		p.EXPECT().Run(gomock.Any()).DoAndReturn(func(ctx context.Context) error { ran <- "poller"; <-ctx.Done(); return nil })
		co.EXPECT().Run(gomock.Any()).DoAndReturn(func(ctx context.Context) error { ran <- "coordinator"; <-ctx.Done(); return errors.New("late") })
		d.core.AddPoller("tuya", p)
		d.core.AddCoordinator(co)

		if err := d.core.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := d.core.Start(t.Context()); err == nil {
			t.Error("second Start should fail")
		}
		got := map[string]bool{<-ran: true, <-ran: true}
		if !got["poller"] || !got["coordinator"] {
			t.Errorf("ran = %v", got)
		}
		if err := d.core.Stop(t.Context()); err != nil {
			t.Errorf("Stop = %v", err)
		}
	})
}

func TestCoreStop(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		d := newCore(t)
		if err := d.core.Stop(t.Context()); err != nil {
			t.Errorf("Stop = %v", err)
		}
	})
	t.Run("deadline exceeded", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := newCore(t)
			p := NewMockPoller(d.ctrl)
			release := make(chan struct{})
			p.EXPECT().Run(gomock.Any()).DoAndReturn(func(context.Context) error { <-release; return nil })
			d.core.AddPoller("tuya", p)
			if err := d.core.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			err := d.core.Stop(ctx) // the poller ignores cancellation: the deadline must win
			close(release)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Stop = %v, want deadline exceeded", err)
			}
		})
	})
}

func TestCoreOnce(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		pollErr error
		showErr error
		wantErr []string
	}{
		{name: "all good"},
		{name: "poll fails, render still runs", pollErr: boom, wantErr: []string{"source tuya: boom"}},
		{name: "render fails", showErr: boom, wantErr: []string{"display hall: boom"}},
		{name: "both fail", pollErr: boom, showErr: boom, wantErr: []string{"source tuya: boom", "display hall: boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newCore(t)
			p := NewMockPoller(d.ctrl)
			co := namedCoordinator(d.ctrl, "hall")
			gomock.InOrder(
				p.EXPECT().Once(gomock.Any()).Return(tt.pollErr),
				co.EXPECT().Render(gomock.Any()).Return(tt.showErr),
			)
			d.core.AddPoller("tuya", p)
			d.core.AddCoordinator(co)
			err := d.core.Once(t.Context())
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Once = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Once returned nil")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

func TestCoreNextPage(t *testing.T) {
	d := newCore(t)
	co := namedCoordinator(d.ctrl, "hall")
	co.EXPECT().NextPage()
	d.core.AddCoordinator(co)
	if err := d.core.NextPage("hall"); err != nil {
		t.Errorf("NextPage = %v", err)
	}
	if err := d.core.NextPage("kitchen"); err == nil || !strings.Contains(err.Error(), `unknown display "kitchen"`) {
		t.Errorf("NextPage(unknown) = %v", err)
	}
}

func TestCoreRefresh(t *testing.T) {
	d := newCore(t)
	co := namedCoordinator(d.ctrl, "hall")
	co.EXPECT().Refresh()
	d.core.AddCoordinator(co)
	if err := d.core.Refresh("hall"); err != nil {
		t.Errorf("Refresh = %v", err)
	}
	if err := d.core.Refresh("kitchen"); err == nil {
		t.Error("Refresh(unknown) should fail")
	}
}

func TestCoreLaterCommands(t *testing.T) {
	d := newCore(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "Acknowledge", call: func() error { return d.core.Acknowledge(1) }},
		{name: "AcknowledgeAll", call: d.core.AcknowledgeAll},
		{name: "Mute", call: func() error { return d.core.Mute(time.Hour) }},
		{name: "Reload", call: d.core.Reload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrNotImplemented) {
				t.Errorf("%s = %v, want ErrNotImplemented", tt.name, err)
			}
		})
	}
}

func TestCoreSetStore(t *testing.T) {
	lock := model.Device{ID: "aqara:lock", Name: "Lock", Kind: model.KindLock}
	reading := func(v bool, at time.Time) model.Reading {
		return model.Reading{Device: lock.ID, Metric: model.Locked, Value: model.BoolValue(v), At: at}
	}
	t.Run("changes, heartbeats and events reach the store in one call", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			d := newCore(t)
			st := NewMockStore(d.ctrl)
			d.core.SetStore(st)
			d.metrics.EXPECT().IncEvent(gomock.Any()).AnyTimes()
			gomock.InOrder(
				// first sight: device, reading, no event
				st.EXPECT().Persist(gomock.Any(), []model.Device{lock}, []model.Reading{reading(true, start)}, nil, start).Return(nil),
				// unchanged within the heartbeat: nothing to write, no call
				// changed: the reading and the unlocked event
				st.EXPECT().Persist(gomock.Any(), nil, []model.Reading{reading(false, start.Add(2*time.Minute))}, gomock.Len(1), start.Add(2*time.Minute)).Return(nil),
				// unchanged but the heartbeat elapsed: the value again, stamped now
				st.EXPECT().Persist(gomock.Any(), nil, []model.Reading{reading(false, start.Add(2*time.Minute+DefaultHeartbeat))}, nil, start.Add(2*time.Minute+DefaultHeartbeat)).Return(nil),
			)
			d.core.Batch("aqara", source.Batch{Devices: []model.Device{lock}, Readings: []model.Reading{reading(true, start)}})
			time.Sleep(time.Minute)
			d.core.Batch("aqara", source.Batch{Readings: []model.Reading{reading(true, start)}})
			time.Sleep(time.Minute)
			d.core.Batch("aqara", source.Batch{Readings: []model.Reading{reading(false, start.Add(2*time.Minute))}})
			time.Sleep(DefaultHeartbeat)
			d.core.Batch("aqara", source.Batch{Readings: []model.Reading{reading(false, start.Add(2*time.Minute))}})
		})
	})
	t.Run("nothing is persisted while the clock is behind the floor", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := newCore(t)
			d.core.cfg.ClockFloor = time.Now().Add(time.Hour)
			st := NewMockStore(d.ctrl) // no expectations: a call would fail the test
			d.core.SetStore(st)
			d.core.Batch("aqara", source.Batch{Devices: []model.Device{lock}, Readings: []model.Reading{reading(true, time.Now())}})
		})
	})
	t.Run("a persist error is logged and counted, not fatal", func(t *testing.T) {
		d := newCore(t)
		st := NewMockStore(d.ctrl)
		d.core.SetStore(st)
		st.EXPECT().Persist(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("disk full"))
		d.metrics.EXPECT().IncStoreError("persist")
		d.metrics.EXPECT().IncEvent("button_pressed")
		d.core.Event(model.Event{Device: lock.ID, Kind: model.EventButtonPressed, At: time.Now()})
	})
	t.Run("a failed write leaves the heartbeat due", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			d := newCore(t)
			st := NewMockStore(d.ctrl)
			d.core.SetStore(st)
			d.metrics.EXPECT().IncStoreError("persist")
			gomock.InOrder(
				st.EXPECT().Persist(gomock.Any(), []model.Device{lock}, []model.Reading{reading(true, start)}, nil, start).Return(errors.New("disk stalled")),
				// nothing was stamped, so the unchanged value is written again as a heartbeat at once
				st.EXPECT().Persist(gomock.Any(), nil, []model.Reading{reading(true, start.Add(time.Minute))}, nil, start.Add(time.Minute)).Return(nil),
			)
			d.core.Batch("aqara", source.Batch{Devices: []model.Device{lock}, Readings: []model.Reading{reading(true, start)}})
			time.Sleep(time.Minute)
			d.core.Batch("aqara", source.Batch{Readings: []model.Reading{reading(true, start)}})
		})
	})
	t.Run("Start restores the configured sources and prunes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := newCore(t)
			st := NewMockStore(d.ctrl)
			d.core.SetStore(st)
			p := NewMockPoller(d.ctrl)
			p.EXPECT().Run(gomock.Any()).DoAndReturn(func(ctx context.Context) error { <-ctx.Done(); return nil })
			d.core.AddPoller("aqara", p)
			old := reading(true, time.Now().Add(-time.Hour))
			gone := model.Device{ID: "tuya:old", Name: "Old"} // its source is no longer configured
			st.EXPECT().Restore(gomock.Any()).Return([]model.Device{lock, gone}, []model.Reading{old,
				{Device: gone.ID, Metric: model.Temperature, Value: model.NumberValue(20), At: old.At}}, nil)
			st.EXPECT().Prune(gomock.Any(), gomock.Any()).Return(int64(3), nil)
			ctx, cancel := context.WithCancel(t.Context())
			if err := d.core.Start(ctx); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			sn := d.state.Snapshot(time.Now())
			if r, ok := sn.Reading(lock.ID, model.Locked); !ok || r != old || !sn.Restored {
				t.Errorf("restored reading = %+v, %t, restored %t", r, ok, sn.Restored)
			}
			if _, ok := sn.Device(gone.ID); ok {
				t.Error("a device of an unconfigured source was restored")
			}
			if _, ok := sn.Reading(gone.ID, model.Temperature); ok {
				t.Error("a reading of an unconfigured source was restored")
			}
			cancel()
			if err := d.core.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	})
}
