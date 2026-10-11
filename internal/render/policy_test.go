package render

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/display"
)

// partialSurface is a 16×2 three-colour panel with 8-pixel windows.
var partialSurface = display.Surface{
	Bounds:      image.Rect(0, 0, 16, 2),
	Palette:     display.ThreeColour(),
	Partial:     true,
	WindowAlign: 8,
	FullRefresh: 19 * time.Second,
}

// instantSurface is a display with no refresh cost, such as a file.
var instantSurface = display.Surface{
	Bounds:  partialSurface.Bounds,
	Palette: partialSurface.Palette,
	Partial: true,
}

// Palette indexes of display.ThreeColour.
const (
	white uint8 = iota
	black
	red
)

// dot is one inked pixel.
type dot struct {
	x, y int
	c    uint8
}

// paint returns a white frame of the surface with the given dots.
func paint(s display.Surface, dots ...dot) *image.Paletted {
	img := image.NewPaletted(s.Bounds, s.Palette)
	for _, d := range dots {
		img.SetColorIndex(d.x, d.y, d.c)
	}
	return img
}

// clockAt returns 2026-10-11 plus days, at the given wall-clock time.
func clockAt(days, hour, minute int) time.Time {
	return time.Date(2026, 10, 11+days, hour, minute, 0, 0, time.UTC)
}

// shown is a decision that reached the glass at a time.
type shown struct {
	mode display.Mode
	at   time.Time
}

func testPolicyConfig() PolicyConfig {
	return PolicyConfig{
		Tick:            time.Minute,
		FullEvery:       time.Hour,
		MinFullInterval: 3 * time.Minute,
		MaxPartials:     2,
		Fast:            true,
		Quiet:           Quiet{From: 22 * time.Hour, To: 8 * time.Hour, Tick: 5 * time.Minute, Wash: 4 * time.Hour},
	}
}

// replay builds a policy and feeds it the given history.
func replay(t *testing.T, cfg PolicyConfig, history []shown) *Policy {
	t.Helper()
	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range history {
		p.Shown(Decision{Mode: h.mode}, h.at)
	}
	return p
}

func TestNewPolicy(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(c *PolicyConfig)
		wantErr string
	}{
		{
			name: "valid",
			edit: func(*PolicyConfig) {},
		},
		{
			name: "no quiet hours",
			edit: func(c *PolicyConfig) { c.Quiet = Quiet{} },
		},
		{
			name: "zero spacing and partials",
			edit: func(c *PolicyConfig) { c.MinFullInterval, c.MaxPartials = 0, 0 },
		},
		{
			name:    "tick",
			edit:    func(c *PolicyConfig) { c.Tick = 0 },
			wantErr: "tick must be positive",
		},
		{
			name:    "full_every",
			edit:    func(c *PolicyConfig) { c.FullEvery = 0 },
			wantErr: "full_every must be positive",
		},
		{
			name:    "negative min_full_interval",
			edit:    func(c *PolicyConfig) { c.MinFullInterval = -time.Second },
			wantErr: "min_full_interval must not be negative",
		},
		{
			name:    "min_full_interval above full_every",
			edit:    func(c *PolicyConfig) { c.MinFullInterval = 2 * time.Hour },
			wantErr: "exceeds full_every",
		},
		{
			name:    "negative max_partials",
			edit:    func(c *PolicyConfig) { c.MaxPartials = -1 },
			wantErr: "max_partials must not be negative",
		},
		{
			name:    "quiet tick",
			edit:    func(c *PolicyConfig) { c.Quiet.Tick = 0 },
			wantErr: "quiet: tick must be positive",
		},
		{
			name:    "time of day out of range",
			edit:    func(c *PolicyConfig) { c.Quiet.To = 25 * time.Hour },
			wantErr: "out of range",
		},
		{
			name:    "wash outside",
			edit:    func(c *PolicyConfig) { c.Quiet.Wash = 12 * time.Hour },
			wantErr: "wash 12h0m0s outside the quiet hours",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testPolicyConfig()
			tt.edit(&cfg)
			p, err := NewPolicy(cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || p == nil {
				t.Fatalf("NewPolicy = %v, %v", p, err)
			}
		})
	}
}

func TestPolicyDecide(t *testing.T) {
	noon := clockAt(0, 12, 0)
	blank := paint(partialSurface)
	full := []shown{{display.ModeFull, noon}}
	tests := []struct {
		name    string
		edit    func(c *PolicyConfig) // nil: testPolicyConfig as is
		history []shown
		surface display.Surface // zero: partialSurface
		last    *image.Paletted
		next    *image.Paletted
		now     time.Time
		urgent  bool
		want    Decision
	}{
		{
			name: "first frame",
			next: blank,
			now:  noon,
			want: Decision{Mode: display.ModeFull, Reason: "first frame"},
		},
		{
			name:    "unknown glass ignores the spacing",
			history: full,
			next:    blank,
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModeFull, Reason: "first frame"},
		},
		{
			name:    "geometry changed",
			history: full,
			last:    paint(display.Surface{Bounds: image.Rect(0, 0, 8, 2), Palette: display.ThreeColour()}),
			next:    blank,
			now:     noon.Add(time.Hour),
			want:    Decision{Mode: display.ModeFull, Reason: "geometry changed"},
		},
		{
			name:    "unchanged",
			history: full,
			last:    blank,
			next:    paint(partialSurface),
			now:     noon.Add(59 * time.Minute),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
		{
			name:    "unchanged but full due",
			history: full,
			last:    blank,
			next:    blank,
			now:     noon.Add(time.Hour),
			want:    Decision{Mode: display.ModeFast, Reason: "full due"},
		},
		{
			name:    "small black change is a partial on aligned edges",
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, black}),
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(8, 1, 16, 2), Reason: "partial"},
		},
		{
			name:    "erasing black is a partial too",
			history: full,
			last:    paint(partialSurface, dot{3, 0, black}, dot{4, 1, black}),
			next:    blank,
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(0, 0, 8, 2), Reason: "partial"},
		},
		{
			name:    "no alignment keeps the box",
			history: full,
			surface: display.Surface{Bounds: partialSurface.Bounds, Palette: partialSurface.Palette, Partial: true},
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, black}, dot{10, 1, black}),
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(9, 1, 11, 2), Reason: "partial"},
		},
		{
			name:    "red changed",
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     noon.Add(3 * time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "red changed"},
		},
		{
			name:    "red erased",
			history: full,
			last:    paint(partialSurface, dot{9, 1, red}),
			next:    blank,
			now:     noon.Add(3 * time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "red changed"},
		},
		{
			name:    "red changed too soon",
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     noon.Add(2 * time.Minute),
			want:    Decision{Skip: true, Reason: "too soon"},
		},
		{
			name:    "an instant display is never too soon",
			history: full,
			surface: instantSurface,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "red changed"},
		},
		{
			name:    "urgent ignores the spacing and is full",
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     noon.Add(time.Minute),
			urgent:  true,
			want:    Decision{Mode: display.ModeFull, Reason: "red changed, urgent"},
		},
		{
			name:    "fast disabled",
			edit:    func(c *PolicyConfig) { c.Fast = false },
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     noon.Add(time.Hour),
			want:    Decision{Mode: display.ModeFull, Reason: "red changed"},
		},
		{
			name:    "no partial mode",
			history: full,
			surface: display.Surface{Bounds: partialSurface.Bounds, Palette: partialSurface.Palette},
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, black}),
			now:     noon.Add(5 * time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "no partial mode"},
		},
		{
			name: "partials exhausted",
			history: []shown{
				{display.ModeFull, noon},
				{display.ModePartial, noon.Add(time.Minute)},
				{display.ModePartial, noon.Add(2 * time.Minute)},
			},
			last: blank,
			next: paint(partialSurface, dot{9, 1, black}),
			now:  noon.Add(3 * time.Minute),
			want: Decision{Mode: display.ModeFast, Reason: "partials exhausted"},
		},
		{
			name:    "zero max_partials never goes partial",
			edit:    func(c *PolicyConfig) { c.MaxPartials = 0 },
			history: full,
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, black}),
			now:     noon.Add(3 * time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "partials exhausted"},
		},
		{
			name:    "red inside the window",
			history: full,
			last:    paint(partialSurface, dot{0, 1, red}),
			next:    paint(partialSurface, dot{0, 1, red}, dot{3, 1, black}),
			now:     noon.Add(3 * time.Minute),
			want:    Decision{Mode: display.ModeFast, Reason: "red in window"},
		},
		{
			name:    "red in another row stays outside the window",
			history: full,
			last:    paint(partialSurface, dot{0, 0, red}),
			next:    paint(partialSurface, dot{0, 0, red}, dot{3, 1, black}),
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(0, 1, 8, 2), Reason: "partial"},
		},
		{
			name:    "red outside the window",
			history: full,
			last:    paint(partialSurface, dot{0, 0, red}),
			next:    paint(partialSurface, dot{0, 0, red}, dot{12, 0, black}),
			now:     noon.Add(time.Minute),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(8, 0, 16, 1), Reason: "partial"},
		},
		{
			name:    "quiet hours: no fast",
			history: []shown{{display.ModeFull, clockAt(0, 22, 0)}},
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     clockAt(0, 23, 0),
			want:    Decision{Mode: display.ModeFull, Reason: "red changed, quiet"},
		},
		{
			name:    "quiet hours: partials still allowed",
			history: []shown{{display.ModeFull, clockAt(0, 23, 0)}},
			last:    blank,
			next:    paint(partialSurface, dot{1, 1, black}),
			now:     clockAt(0, 23, 5),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(0, 1, 8, 2), Reason: "partial"},
		},
		{
			name:    "wash due",
			history: []shown{{display.ModeFull, clockAt(1, 3, 30)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 4, 0),
			want:    Decision{Mode: display.ModeWash, Reason: "wash"},
		},
		{
			name:    "wash due later in the night",
			history: []shown{{display.ModeFull, clockAt(1, 3, 30)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 7, 55),
			want:    Decision{Mode: display.ModeWash, Reason: "wash"},
		},
		{
			name:    "wash not yet",
			history: []shown{{display.ModeFull, clockAt(1, 3, 30)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 3, 55),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
		{
			name:    "wash too soon is deferred",
			history: []shown{{display.ModeFull, clockAt(1, 3, 58)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 4, 0),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
		{
			name:    "wash too soon still lets a partial through",
			history: []shown{{display.ModeFull, clockAt(1, 3, 58)}},
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, black}),
			now:     clockAt(1, 4, 0),
			want:    Decision{Mode: display.ModePartial, Window: image.Rect(8, 1, 16, 2), Reason: "partial"},
		},
		{
			name:    "deferred wash happens on the next tick",
			history: []shown{{display.ModeFull, clockAt(1, 3, 58)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 4, 5),
			want:    Decision{Mode: display.ModeWash, Reason: "wash"},
		},
		{
			name:    "urgent does not wash",
			history: []shown{{display.ModeFull, clockAt(1, 3, 30)}},
			last:    blank,
			next:    paint(partialSurface, dot{9, 1, red}),
			now:     clockAt(1, 4, 0),
			urgent:  true,
			want:    Decision{Mode: display.ModeFull, Reason: "red changed, urgent"},
		},
		{
			name:    "washed this night already",
			history: []shown{{display.ModeWash, clockAt(1, 4, 0)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 4, 5),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
		{
			name:    "next night washes again",
			history: []shown{{display.ModeWash, clockAt(1, 4, 0)}, {display.ModeFull, clockAt(1, 23, 0)}},
			last:    blank,
			next:    blank,
			now:     clockAt(2, 4, 0),
			want:    Decision{Mode: display.ModeWash, Reason: "wash"},
		},
		{
			name:    "wash before midnight is not repeated after it",
			edit:    func(c *PolicyConfig) { c.FullEvery, c.Quiet.Wash = 3*time.Hour, 23*time.Hour },
			history: []shown{{display.ModeWash, clockAt(0, 23, 0)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 1, 0),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
		{
			name:    "no quiet hours, no wash",
			edit:    func(c *PolicyConfig) { c.Quiet = Quiet{} },
			history: []shown{{display.ModeFull, clockAt(1, 3, 30)}},
			last:    blank,
			next:    blank,
			now:     clockAt(1, 4, 0),
			want:    Decision{Skip: true, Reason: "unchanged"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testPolicyConfig()
			if tt.edit != nil {
				tt.edit(&cfg)
			}
			p := replay(t, cfg, tt.history)
			s := tt.surface
			if s.Bounds.Empty() {
				s = partialSurface
			}
			if got := p.Decide(tt.last, tt.next, s, tt.now, tt.urgent); got != tt.want {
				t.Errorf("Decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPolicyShown(t *testing.T) {
	noon := clockAt(0, 12, 0)
	type book struct {
		lastColour time.Time
		partials   int
		lastWash   time.Time
	}
	tests := []struct {
		name    string
		history []shown
		skip    bool // the last entry is a skip
		want    book
	}{
		{
			name: "nothing yet",
			want: book{},
		},
		{
			name:    "full",
			history: []shown{{display.ModeFull, noon}},
			want:    book{lastColour: noon},
		},
		{
			name: "partials count",
			history: []shown{
				{display.ModeFull, noon},
				{display.ModePartial, noon.Add(time.Minute)},
				{display.ModePartial, noon.Add(2 * time.Minute)},
			},
			want: book{lastColour: noon, partials: 2},
		},
		{
			name: "fast resets the partials",
			history: []shown{
				{display.ModeFull, noon},
				{display.ModePartial, noon.Add(time.Minute)},
				{display.ModeFast, noon.Add(5 * time.Minute)},
			},
			want: book{lastColour: noon.Add(5 * time.Minute)},
		},
		{
			name:    "a skip records nothing",
			history: []shown{{display.ModeFull, noon}, {display.ModeFast, noon.Add(time.Hour)}},
			skip:    true,
			want:    book{lastColour: noon},
		},
		{
			name:    "wash names its night",
			history: []shown{{display.ModeWash, clockAt(1, 4, 0)}},
			want:    book{lastColour: clockAt(1, 4, 0), lastWash: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := replay(t, testPolicyConfig(), nil)
			for i, h := range tt.history {
				p.Shown(Decision{Mode: h.mode, Skip: tt.skip && i == len(tt.history)-1}, h.at)
			}
			if got := (book{p.lastColour, p.partials, p.lastWash}); got != tt.want {
				t.Errorf("bookkeeping = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPolicyFailed(t *testing.T) {
	noon := clockAt(0, 12, 0)
	tests := []struct {
		name    string
		history []shown
		want    int
	}{
		{
			name: "nothing shown",
			want: 0,
		},
		{
			name:    "partials forgotten",
			history: []shown{{display.ModeFull, noon}, {display.ModePartial, noon.Add(time.Minute)}},
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := replay(t, testPolicyConfig(), tt.history)
			p.Failed()
			if p.partials != tt.want {
				t.Errorf("partials = %d, want %d", p.partials, tt.want)
			}
			// The coordinator forgets the glass: the next frame is full.
			if got := p.Decide(
				nil,
				paint(partialSurface),
				partialSurface,
				noon.Add(2*time.Minute),
				false,
			); got.Mode != display.ModeFull || got.Skip {
				t.Errorf("Decide after failure = %+v, want full", got)
			}
		})
	}
}

func TestPolicyTick(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		now   time.Time
		want  time.Duration
	}{
		{name: "day", quiet: true, now: clockAt(0, 12, 0), want: time.Minute},
		{name: "evening, before quiet hours", quiet: true, now: clockAt(0, 21, 59), want: time.Minute},
		{name: "quiet hours start", quiet: true, now: clockAt(0, 22, 0), want: 5 * time.Minute},
		{name: "after midnight", quiet: true, now: clockAt(1, 3, 0), want: 5 * time.Minute},
		{name: "quiet hours end", quiet: true, now: clockAt(1, 8, 0), want: time.Minute},
		{name: "no quiet hours", now: clockAt(1, 3, 0), want: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testPolicyConfig()
			if !tt.quiet {
				cfg.Quiet = Quiet{}
			}
			if got := replay(t, cfg, nil).Tick(tt.now); got != tt.want {
				t.Errorf("Tick(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}
