package render

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/scene"
	"github.com/mshaulsky/domovoi/internal/state"
)

var testSurface = display.Surface{Bounds: image.Rect(0, 0, 4, 2), Palette: display.ThreeColour()}

// quiet is a logger that keeps test output clean.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// frame returns a 4×2 frame with the given pixel inked, so tests can make
// frames that differ.
func frame(inked int) *image.Paletted {
	img := image.NewPaletted(testSurface.Bounds, testSurface.Palette)
	img.Pix[inked] = 1
	return img
}

func bundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func quietMetrics(ctrl *gomock.Controller) *MockMetrics {
	m := NewMockMetrics(ctrl)
	m.EXPECT().IncRender(gomock.Any(), gomock.Any()).AnyTimes()
	m.EXPECT().ObserveRenderDuration(gomock.Any(), gomock.Any()).AnyTimes()
	m.EXPECT().IncRenderError(gomock.Any()).AnyTimes()
	return m
}

func sceneNamed(ctrl *gomock.Controller, name string) *MockScene {
	sc := NewMockScene(ctrl)
	sc.EXPECT().Name().Return(name).AnyTimes()
	return sc
}

type deps struct {
	ctrl    *gomock.Controller
	disp    *MockDisplay
	scenes  []*MockScene
	state   *state.State
	metrics *MockMetrics
	locale  Locale  // nil: English, UTC
	history History // nil: no storage
}

func newDeps(t *testing.T, sceneCount int) deps {
	t.Helper()
	ctrl := gomock.NewController(t)
	d := deps{ctrl: ctrl, disp: NewMockDisplay(ctrl), state: state.New(), metrics: quietMetrics(ctrl)}
	d.disp.EXPECT().Surface().Return(testSurface).AnyTimes()
	for i := range sceneCount {
		d.scenes = append(d.scenes, sceneNamed(ctrl, "scene"+string(rune('a'+i))))
	}
	return d
}

func (d deps) coordinator(t *testing.T, cfg Config) *Coordinator {
	t.Helper()
	if cfg.Name == "" {
		cfg.Name = "hall"
	}
	if cfg.Tick == 0 {
		cfg.Tick = 5 * time.Minute
	}
	if cfg.FullEvery == 0 {
		cfg.FullEvery = time.Hour
	}
	locale := d.locale
	if locale == nil {
		locale = NewFixedLocale(bundle(t), nil)
	}
	scenes := make([]Scene, len(d.scenes))
	for i, sc := range d.scenes {
		scenes[i] = sc
	}
	c, err := New(cfg, d.disp, scenes, d.state, locale, d.history, quiet, d.metrics)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNew(t *testing.T) {
	d := newDeps(t, 1)
	english := NewFixedLocale(bundle(t), nil)
	tests := []struct {
		name    string
		cfg     Config
		scenes  int
		locale  Locale
		wantErr string
	}{
		{name: "valid", cfg: Config{Name: "hall", Tick: time.Minute}, scenes: 1, locale: english},
		{name: "name required", cfg: Config{Tick: time.Minute}, scenes: 1, locale: english, wantErr: "name is required"},
		{name: "tick required", cfg: Config{Name: "hall"}, scenes: 1, locale: english, wantErr: "tick must be positive"},
		{name: "scene required", cfg: Config{Name: "hall", Tick: time.Minute}, scenes: 0, locale: english, wantErr: "at least one scene"},
		{name: "locale required", cfg: Config{Name: "hall", Tick: time.Minute}, scenes: 1, wantErr: "locale is required"},
		{name: "bundle required", cfg: Config{Name: "hall", Tick: time.Minute}, scenes: 1, locale: NewFixedLocale(nil, nil), wantErr: "language bundle is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var scenes []Scene
			for range tt.scenes {
				scenes = append(scenes, d.scenes[0])
			}
			c, err := New(tt.cfg, d.disp, scenes, d.state, tt.locale, nil, quiet, d.metrics)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.locale.Location() != time.UTC || c.cfg.StartGrace != DefaultStartGrace {
				t.Errorf("defaults not applied: %+v", c.cfg)
			}
		})
	}
}

func TestCoordinatorName(t *testing.T) {
	d := newDeps(t, 1)
	if got := d.coordinator(t, Config{Name: "kitchen"}).Name(); got != "kitchen" {
		t.Errorf("Name() = %q", got)
	}
}

func TestCoordinatorRender(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		setup   func(t *testing.T, d deps, c *Coordinator)
		wantErr string
	}{
		{
			name: "first frame is shown in full",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil)
				d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, f display.Frame) error {
					if f.Mode != display.ModeFull || f.Image == nil {
						t.Errorf("frame = %+v, want full with image", f)
					}
					return nil
				})
			},
		},
		{
			name: "unchanged frame is skipped",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil).Times(2)
				d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				if err := c.Render(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "changed frame is shown",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				gomock.InOrder(
					d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil),
					d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(1), nil),
				)
				d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				if err := c.Render(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unchanged frame redrawn after full_every",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil).Times(2)
				d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).Times(2)
				if err := c.Render(t.Context()); err != nil {
					t.Fatal(err)
				}
				time.Sleep(time.Hour)
			},
		},
		{
			name: "scene error",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(nil, boom)
			},
			wantErr: "render hall/scenea: boom",
		},
		{
			name: "show error forgets the glass",
			setup: func(t *testing.T, d deps, c *Coordinator) {
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil).Times(3)
				gomock.InOrder(
					d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil),
					d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(boom),
					d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil), // same pixels, shown again: the glass was unknown
				)
				if err := c.Render(t.Context()); err != nil {
					t.Fatal(err)
				}
				c.forget() // pretend the panel was touched; the second show then fails
				if err := c.Render(t.Context()); err == nil {
					t.Fatal("second render should fail")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				d := newDeps(t, 1)
				c := d.coordinator(t, Config{})
				tt.setup(t, d, c)
				err := c.Render(t.Context())
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
					}
					if !errors.Is(err, boom) {
						t.Error("error does not wrap the cause")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestCoordinatorRenderView(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		d := newDeps(t, 1)
		d.state.ApplyDevices([]model.Device{{ID: "tuya:a", Name: "A"}})
		d.state.ApplyReadings([]model.Reading{{Device: "tuya:a", Metric: model.Temperature, Value: model.NumberValue(21), At: now}})
		d.state.SetHealth("tuya", nil, now)
		d.state.SetHealth("aqara", errors.New("down"), now)
		almaty := time.FixedZone("Asia/Almaty", 5*3600)
		d.locale = NewFixedLocale(bundle(t), almaty)
		h := NewMockHistory(d.ctrl)
		d.history = h
		h.EXPECT().Extremes(gomock.Any(), model.DeviceID("tuya:a"), model.Temperature, gomock.Any()).Return(19.5, 22.0, true, nil)
		h.EXPECT().Trend(gomock.Any(), model.DeviceID("tuya:a"), model.Temperature, now.Add(-trendWindow)).Return(0.4, true, nil)
		h.EXPECT().Events(gomock.Any(), now.Add(-eventsWindow), eventsLimit).Return([]model.Event{{Device: "tuya:a", Kind: model.EventOnline, At: now}}, nil)
		c := d.coordinator(t, Config{})
		var got scene.View
		d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).DoAndReturn(func(_ display.Surface, v scene.View) (*image.Paletted, error) {
			got = v
			return frame(0), nil
		})
		d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil)
		if err := c.Render(t.Context()); err != nil {
			t.Fatal(err)
		}
		if !got.Now.Equal(now) || got.Location != almaty || got.Bundle == nil || got.Starting {
			t.Errorf("view basics = now %v loc %v starting %t", got.Now, got.Location, got.Starting)
		}
		if len(got.Devices) != 1 || got.Stale["tuya:a"] || !got.Seen["tuya:a"].Equal(now) {
			t.Errorf("devices = %v stale = %v seen = %v", got.Devices, got.Stale, got.Seen)
		}
		if r, ok := got.Reading("tuya:a", model.Temperature); !ok || r.Value.Num != 21 {
			t.Errorf("reading = %+v, %t", r, ok)
		}
		if ex := got.Extremes["tuya:a"][model.Temperature]; ex.Min != 19.5 || ex.Max != 22 || got.Trends["tuya:a"][model.Temperature] != 0.4 || len(got.Events) != 1 {
			t.Errorf("history = extremes %+v trends %v events %v", ex, got.Trends, got.Events)
		}
		if len(got.Sources) != 2 || got.Sources[0].Name != "aqara" || !got.Sources[0].Stale || got.Sources[1].Name != "tuya" || got.Sources[1].Stale {
			t.Errorf("sources = %+v", got.Sources)
		}
	})
}

func TestCoordinatorRenderStarting(t *testing.T) {
	tests := []struct {
		name  string
		setup func(st *state.State, now time.Time)
		want  bool
	}{
		{name: "no sources configured", setup: func(*state.State, time.Time) {}, want: false},
		{name: "configured, none answered", setup: func(st *state.State, _ time.Time) { st.SetStaleAfter("tuya", time.Minute) }, want: true},
		{name: "one failed", setup: func(st *state.State, now time.Time) {
			st.SetStaleAfter("tuya", time.Minute)
			st.SetHealth("tuya", errors.New("down"), now)
		}, want: false},
		{name: "one succeeded", setup: func(st *state.State, now time.Time) {
			st.SetStaleAfter("tuya", time.Minute)
			st.SetStaleAfter("aqara", time.Minute)
			st.SetHealth("tuya", nil, now)
		}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				d := newDeps(t, 1)
				tt.setup(d.state, time.Now())
				c := d.coordinator(t, Config{})
				var got scene.View
				d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).DoAndReturn(func(_ display.Surface, v scene.View) (*image.Paletted, error) {
					got = v
					return frame(0), nil
				})
				d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil)
				if err := c.Render(t.Context()); err != nil {
					t.Fatal(err)
				}
				if got.Starting != tt.want {
					t.Errorf("Starting = %t, want %t", got.Starting, tt.want)
				}
			})
		})
	}
}

func TestCoordinatorRefreshDuringShow(t *testing.T) {
	// A Refresh that arrives while Show is running must not be lost: the
	// next render shows the same pixels again.
	synctest.Test(t, func(t *testing.T) {
		d := newDeps(t, 1)
		c := d.coordinator(t, Config{})
		d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil).Times(2)
		gomock.InOrder(
			d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, display.Frame) error {
				c.Refresh() // the button was pressed mid-frame
				return nil
			}),
			d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil),
		)
		for range 2 {
			if err := c.Render(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestCoordinatorRun(t *testing.T) {
	t.Run("renders after the start grace and on ticks", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now() // midnight: on a tick boundary
			d := newDeps(t, 1)
			c := d.coordinator(t, Config{StartGrace: 30 * time.Second})
			rendered := make(chan time.Time, 8)
			d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).DoAndReturn(func(_ display.Surface, v scene.View) (*image.Paletted, error) {
				rendered <- v.Now
				return frame(len(rendered) % 8), nil
			}).AnyTimes()
			d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()

			time.Sleep(29 * time.Second)
			synctest.Wait()
			if len(rendered) != 0 {
				t.Fatal("rendered before the start grace")
			}
			time.Sleep(time.Second)
			if got := <-rendered; !got.Equal(start.Add(30 * time.Second)) {
				t.Errorf("first render at %v, want the start grace", got)
			}
			time.Sleep(4*time.Minute + 29*time.Second)
			synctest.Wait()
			if len(rendered) != 0 {
				t.Fatal("rendered before the tick")
			}
			time.Sleep(time.Second)
			if got := <-rendered; !got.Equal(start.Add(5 * time.Minute)) {
				t.Errorf("tick render at %v, want the 5-minute boundary", got)
			}
			cancel()
			if err := <-done; err != nil {
				t.Errorf("Run = %v", err)
			}
		})
	})
	t.Run("wake renders immediately", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := newDeps(t, 1)
			c := d.coordinator(t, Config{})
			rendered := make(chan struct{}, 8)
			d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).DoAndReturn(func(display.Surface, scene.View) (*image.Paletted, error) {
				rendered <- struct{}{}
				return frame(len(rendered) % 8), nil
			}).AnyTimes()
			d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()
			synctest.Wait()
			c.Wake()
			synctest.Wait()
			if len(rendered) != 1 {
				t.Fatalf("Wake rendered %d times, want 1", len(rendered))
			}
			cancel()
			<-done
		})
	})
	t.Run("render errors are logged, not fatal", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := newDeps(t, 1)
			c := d.coordinator(t, Config{})
			d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(nil, errors.New("boom")).AnyTimes()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()
			synctest.Wait()
			c.Wake()
			synctest.Wait()
			cancel()
			if err := <-done; err != nil {
				t.Errorf("Run = %v", err)
			}
		})
	})
}

func TestCoordinatorWake(t *testing.T) {
	d := newDeps(t, 1)
	c := d.coordinator(t, Config{})
	c.Wake()
	c.Wake() // coalesces instead of blocking
	if len(c.wake) != 1 {
		t.Errorf("pending wakes = %d, want 1", len(c.wake))
	}
}

func TestCoordinatorNextPage(t *testing.T) {
	d := newDeps(t, 2)
	c := d.coordinator(t, Config{})
	gomock.InOrder(
		d.scenes[1].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil),
		d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(1), nil),
	)
	d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	c.NextPage()
	if err := c.Render(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.NextPage() // wraps around
	if err := c.Render(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(c.wake) != 1 {
		t.Error("NextPage did not wake")
	}
}

func TestCoordinatorRefresh(t *testing.T) {
	d := newDeps(t, 1)
	c := d.coordinator(t, Config{})
	d.scenes[0].EXPECT().Render(testSurface, gomock.Any()).Return(frame(0), nil).Times(2)
	d.disp.EXPECT().Show(gomock.Any(), gomock.Any()).Return(nil).Times(2) // the same pixels shown twice: Refresh forgot the glass
	if err := c.Render(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.Refresh()
	if err := c.Render(t.Context()); err != nil {
		t.Fatal(err)
	}
}
