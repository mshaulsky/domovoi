package epd7in5b

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/domovoi/internal/display"
)

// recorder keeps the order of the calls that matter across the panel and
// the port.
type recorder struct {
	calls []string
}

// port is a fake io.Closer recording Close.
type port struct {
	rec *recorder
	err error
}

func (p *port) Close() error {
	p.rec.calls = append(p.rec.calls, "close")
	return p.err
}

var (
	errDriver = errors.New("spi broke")
	window    = image.Rect(400, 40, 720, 120)
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func frame(mode display.Mode) display.Frame {
	return display.Frame{
		Image:  image.NewPaletted(image.Rect(0, 0, 800, 480), display.ThreeColour()),
		Window: window,
		Mode:   mode,
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		panel   Panel
		port    io.Closer
		log     *slog.Logger
		want    time.Duration
		wantErr string
	}{
		{
			name:  "valid",
			cfg:   Config{Timeout: time.Minute},
			panel: NopPanel{},
			port:  &port{rec: &recorder{}},
			log:   discard(),
			want:  time.Minute,
		},
		{
			name:  "default timeout",
			panel: NopPanel{},
			port:  &port{rec: &recorder{}},
			log:   discard(),
			want:  DefaultTimeout,
		},
		{
			name:  "nil port allowed",
			panel: NopPanel{},
			log:   discard(),
			want:  DefaultTimeout,
		},
		{
			name:    "nil panel",
			log:     discard(),
			wantErr: "panel is required",
		},
		{
			name:    "nil log",
			panel:   NopPanel{},
			wantErr: "logger is required",
		},
		{
			name:    "negative timeout",
			cfg:     Config{Timeout: -time.Second},
			panel:   NopPanel{},
			log:     discard(),
			wantErr: "must not be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := New(tt.cfg, tt.panel, tt.port, tt.log)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.cfg.Timeout != tt.want {
				t.Errorf("timeout = %s, want %s", d.cfg.Timeout, tt.want)
			}
		})
	}
}

func TestDisplaySurface(t *testing.T) {
	d, err := New(Config{}, NopPanel{}, nil, discard())
	if err != nil {
		t.Fatal(err)
	}
	got := d.Surface()
	want := display.Surface{
		Bounds:      image.Rect(0, 0, 800, 480),
		Palette:     display.ThreeColour(),
		Partial:     true,
		WindowAlign: 8,
		FullRefresh: 19 * time.Second,
	}
	if got.Bounds != want.Bounds || !slices.Equal(got.Palette, want.Palette) || got.Partial != want.Partial ||
		got.WindowAlign != want.WindowAlign || got.FullRefresh != want.FullRefresh {
		t.Errorf("Surface() = %+v, want %+v", got, want)
	}
}

func TestDisplayShow(t *testing.T) {
	tests := []struct {
		name      string
		frame     display.Frame
		closed    bool // Close before Show
		cancelled bool // ctx cancelled before Show
		expect    func(t *testing.T, p *MockPanel, f display.Frame)
		want      time.Duration // virtual time Show takes
		wantErr   string
		wantIs    error
	}{
		{
			name:  "full",
			frame: frame(display.ModeFull),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
		},
		{
			name:  "fast",
			frame: frame(display.ModeFast),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().InitFast(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
		},
		{
			name:  "partial",
			frame: frame(display.ModePartial),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().InitPartial(gomock.Any()),
					p.EXPECT().DisplayPartial(gomock.Any(), f.Image, window),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
		},
		{
			name:  "wash",
			frame: frame(display.ModeWash),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Clear(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
		},
		{
			name:    "unknown mode",
			frame:   frame(display.Mode(9)),
			wantErr: "unknown refresh mode 9",
		},
		{
			name:    "nil image",
			frame:   display.Frame{Mode: display.ModeFull},
			wantErr: "frame has no image",
		},
		{
			name: "wrong size",
			frame: display.Frame{
				Image: image.NewPaletted(image.Rect(0, 0, 400, 300), display.ThreeColour()),
				Mode:  display.ModeFull,
			},
			wantErr: "frame bounds (0,0)-(400,300), the panel is (0,0)-(800,480)",
		},
		{
			name: "empty partial window",
			frame: display.Frame{
				Image:  image.NewPaletted(image.Rect(0, 0, 800, 480), display.ThreeColour()),
				Window: image.Rect(900, 0, 1000, 10),
				Mode:   display.ModePartial,
			},
			wantErr: "partial window (900,0)-(1000,10) is empty or outside the panel",
		},
		{
			name:  "display fails",
			frame: frame(display.ModeFull),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image).Return(errDriver),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
			wantErr: "epd7in5b: show full: spi broke",
			wantIs:  errDriver,
		},
		{
			name:  "init fails",
			frame: frame(display.ModeFast),
			expect: func(_ *testing.T, p *MockPanel, _ display.Frame) {
				gomock.InOrder(
					p.EXPECT().InitFast(gomock.Any()).Return(errDriver),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
			wantErr: "epd7in5b: show fast: spi broke",
			wantIs:  errDriver,
		},
		{
			name:  "sleep fails after a good transfer",
			frame: frame(display.ModeFull),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image),
					p.EXPECT().Sleep(gomock.Any()).Return(errDriver),
				)
			},
		},
		{
			name:    "show after close",
			frame:   frame(display.ModeFull),
			closed:  true,
			wantErr: "show after close",
		},
		{
			name:      "cancelled",
			frame:     frame(display.ModeFull),
			cancelled: true,
			expect: func(t *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image).DoAndReturn(
						func(ctx context.Context, _ image.Image) error {
							<-ctx.Done()
							return ctx.Err()
						},
					),
					p.EXPECT().Sleep(gomock.Any()).Do(func(ctx context.Context) {
						if ctx.Err() != nil {
							t.Errorf("Sleep got a done context: %v", ctx.Err())
						}
					}),
				)
			},
			wantErr: "epd7in5b: show full",
			wantIs:  context.Canceled,
		},
		{
			name:  "timeout",
			frame: frame(display.ModeWash),
			expect: func(_ *testing.T, p *MockPanel, f display.Frame) {
				gomock.InOrder(
					p.EXPECT().Init(gomock.Any()),
					p.EXPECT().Clear(gomock.Any()),
					p.EXPECT().Display(gomock.Any(), f.Image).DoAndReturn(
						func(ctx context.Context, _ image.Image) error {
							<-ctx.Done()
							return ctx.Err()
						},
					),
					p.EXPECT().Sleep(gomock.Any()),
				)
			},
			want:    DefaultTimeout,
			wantErr: "epd7in5b: show wash",
			wantIs:  context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := NewMockPanel(gomock.NewController(t))
				if tt.expect != nil {
					tt.expect(t, p, tt.frame)
				}
				d, err := New(Config{}, p, nil, discard())
				if err != nil {
					t.Fatal(err)
				}
				if tt.closed {
					if err := d.Close(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if tt.cancelled {
					cancel()
				}
				start := time.Now()
				err = d.Show(ctx, tt.frame)
				if took := time.Since(start); took != tt.want {
					t.Errorf("Show took %s, want %s", took, tt.want)
				}
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
					}
					if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
						t.Errorf("error = %v, want wrapping %v", err, tt.wantIs)
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

func TestDisplayClose(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, d *Display, p *MockPanel) // leaves the display in the state under test
		expect  func(p *MockPanel, rec *recorder)
		portErr error
		want    []string
		wantErr string
	}{
		{
			name: "asleep",
			want: []string{"close"},
		},
		{
			name:    "awake after a failed sleep",
			prepare: leaveAwake,
			expect: func(p *MockPanel, rec *recorder) {
				p.EXPECT().Sleep(gomock.Any()).Do(func(context.Context) {
					rec.calls = append(rec.calls, "sleep")
				})
			},
			want: []string{"sleep", "close"},
		},
		{
			name:    "sleep and port errors both reported",
			prepare: leaveAwake,
			expect: func(p *MockPanel, rec *recorder) {
				p.EXPECT().Sleep(gomock.Any()).DoAndReturn(func(context.Context) error {
					rec.calls = append(rec.calls, "sleep")
					return errDriver
				})
			},
			portErr: errors.New("port stuck"),
			want:    []string{"sleep", "close"},
			wantErr: "epd7in5b: close: sleep: spi broke\nepd7in5b: close: release port: port stuck",
		},
		{
			name: "second close is a no-op",
			prepare: func(t *testing.T, d *Display, _ *MockPanel) {
				if err := d.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewMockPanel(gomock.NewController(t))
			rec := &recorder{}
			d, err := New(Config{}, p, &port{rec: rec, err: tt.portErr}, discard())
			if err != nil {
				t.Fatal(err)
			}
			if tt.prepare != nil {
				tt.prepare(t, d, p)
			}
			rec.calls = nil
			if tt.expect != nil {
				tt.expect(p, rec)
			}
			err = d.Close(t.Context())
			if !slices.Equal(rec.calls, tt.want) {
				t.Errorf("calls = %v, want %v", rec.calls, tt.want)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("waits for a show in flight", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			p := NewMockPanel(gomock.NewController(t))
			rec := &recorder{}
			d, err := New(Config{}, p, &port{rec: rec}, discard())
			if err != nil {
				t.Fatal(err)
			}
			f := frame(display.ModeFull)
			release := make(chan struct{})
			gomock.InOrder(
				p.EXPECT().Init(gomock.Any()),
				p.EXPECT().Display(gomock.Any(), f.Image).DoAndReturn(func(context.Context, image.Image) error {
					<-release
					return nil
				}),
				p.EXPECT().Sleep(gomock.Any()).Do(func(context.Context) {
					rec.calls = append(rec.calls, "sleep")
				}),
			)
			var wg sync.WaitGroup
			wg.Go(func() {
				if err := d.Show(t.Context(), f); err != nil {
					t.Errorf("Show = %v", err)
				}
			})
			synctest.Wait()
			closed := make(chan struct{})
			wg.Go(func() {
				defer close(closed)
				if err := d.Close(t.Context()); err != nil {
					t.Errorf("Close = %v", err)
				}
			})
			synctest.Wait()
			select {
			case <-closed:
				t.Fatal("Close returned while a Show was in flight")
			default:
			}
			close(release)
			wg.Wait()
			if want := []string{"sleep", "close"}; !slices.Equal(rec.calls, want) {
				t.Errorf("calls = %v, want %v", rec.calls, want)
			}
		})
	})
}

// leaveAwake runs a Show whose Sleep fails, so the panel stays awake.
func leaveAwake(t *testing.T, d *Display, p *MockPanel) {
	t.Helper()
	f := frame(display.ModeFull)
	gomock.InOrder(
		p.EXPECT().Init(gomock.Any()),
		p.EXPECT().Display(gomock.Any(), f.Image),
		p.EXPECT().Sleep(gomock.Any()).Return(errDriver),
	)
	if err := d.Show(t.Context(), f); err != nil {
		t.Fatal(err)
	}
}
