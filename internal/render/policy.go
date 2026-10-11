package render

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/mshaulsky/domovoi/internal/display"
)

// Policy decides how a frame reaches the glass: not at all, a monochrome
// partial update of the changed window, or a colour refresh (Full, Fast or
// the nightly Wash). It is display-agnostic and has no clock: every call
// takes the time, already in the display's zone. Its bookkeeping — the last
// colour refresh, the partial updates since, the night of the last wash —
// changes only through Shown and Failed. A Policy is not safe for
// concurrent use; the coordinator serialises it with its render lock.
type Policy struct {
	cfg        PolicyConfig
	lastColour time.Time // when the last colour refresh started; zero = none yet
	partials   int       // partial updates since lastColour
	lastWash   time.Time // the night of the last wash (see washNight); zero = never
}

// PolicyConfig tunes a policy: the render cadence and the refresh
// decisions. Zero values are the strictest: no minimum spacing of colour
// refreshes, no partial updates, no fast mode, no quiet hours.
type PolicyConfig struct {
	Tick time.Duration // render cadence, aligned to the wall clock
	// the longest gap between colour refreshes; an unchanged frame is still
	// refreshed this often
	FullEvery       time.Duration
	MinFullInterval time.Duration // the shortest gap between colour refreshes, unless urgent
	MaxPartials     int           // partial updates allowed between two colour refreshes
	Fast            bool          // colour refreshes may use the fast waveform outside quiet hours
	Quiet           Quiet         // the night's tick and wash
}

// Decision is how one frame reaches the glass, or that it does not.
type Decision struct {
	Mode   display.Mode
	Window image.Rectangle // the region of a ModePartial update; empty otherwise
	Skip   bool            // nothing is sent this tick
	Reason string          // why, for logs
}

// Quiet is the night: a longer tick, no fast refreshes, and once per night
// a wash. Times are wall-clock offsets from local midnight; the window
// [From, To) may cross midnight. From == To means no quiet hours.
type Quiet struct {
	From time.Duration
	To   time.Duration
	Tick time.Duration // render cadence inside the window
	Wash time.Duration // when the nightly wash is due; inside [From, To)
}

// Decision reasons, for logs.
const (
	reasonUnchanged  = "unchanged"
	reasonFirst      = "first frame"
	reasonGeometry   = "geometry changed"
	reasonWash       = "wash"
	reasonRed        = "red changed"
	reasonFullDue    = "full due"
	reasonNoPartial  = "no partial mode"
	reasonExhausted  = "partials exhausted"
	reasonRedInside  = "red in window"
	reasonPartial    = "partial"
	reasonTooSoon    = "too soon"
	reasonUrgent     = "urgent"
	reasonQuietColor = "quiet"

	day = 24 * time.Hour
)

// accent is the colour a partial update can neither draw nor erase.
var accent = rgba(display.Red)

// NewPolicy validates cfg. MaxPartials 0 means every change is a colour
// refresh; MinFullInterval 0 means no minimum.
func NewPolicy(cfg PolicyConfig) (*Policy, error) {
	switch {
	case cfg.Tick <= 0:
		return nil, errors.New("policy: tick must be positive")
	case cfg.FullEvery <= 0:
		return nil, errors.New("policy: full_every must be positive")
	case cfg.MinFullInterval < 0:
		return nil, errors.New("policy: min_full_interval must not be negative")
	case cfg.MinFullInterval > cfg.FullEvery:
		return nil, fmt.Errorf("policy: min_full_interval %v exceeds full_every %v", cfg.MinFullInterval, cfg.FullEvery)
	case cfg.MaxPartials < 0:
		return nil, errors.New("policy: max_partials must not be negative")
	}
	if err := cfg.Quiet.validate(); err != nil {
		return nil, fmt.Errorf("policy: quiet: %w", err)
	}
	return &Policy{cfg: cfg}, nil
}

// Decide says how next reaches the glass. last is what the glass shows,
// nil when unknown (the first frame, after a failed Show or a Refresh); now
// is in the display's zone; urgent marks a render triggered by an urgent
// alert, which may break the colour-refresh spacing and never waits for the
// wash. An unknown glass is always redrawn in full at once: there is
// nothing on it worth protecting. A wash that falls too close to the last
// colour refresh is deferred to a later tick, and the frame is decided as on
// any other tick meanwhile. The spacing of colour refreshes applies only to
// a surface with a FullRefresh time: an instant display is never too soon.
func (p *Policy) Decide(last, next *image.Paletted, s display.Surface, now time.Time, urgent bool) Decision {
	switch {
	case last == nil:
		return Decision{Mode: display.ModeFull, Reason: reasonFirst}
	case last.Rect != next.Rect:
		return Decision{Mode: display.ModeFull, Reason: reasonGeometry}
	}
	// A wash too close to the last colour refresh waits for a later tick;
	// meanwhile the frame is treated as on any other tick.
	if !urgent && p.washDue(now) && !p.tooSoon(s, now) {
		return Decision{Mode: display.ModeWash, Reason: reasonWash}
	}
	box, red := compare(last, next)
	fullDue := now.Sub(p.lastColour) >= p.cfg.FullEvery
	switch {
	case box.Empty() && !fullDue:
		return Decision{Skip: true, Reason: reasonUnchanged}
	case red:
		return p.colour(reasonRed, s, now, urgent)
	case fullDue:
		return p.colour(reasonFullDue, s, now, urgent)
	case !s.Partial:
		return p.colour(reasonNoPartial, s, now, urgent)
	case p.partials >= p.cfg.MaxPartials:
		return p.colour(reasonExhausted, s, now, urgent)
	}
	window := align(box, s).Intersect(next.Rect)
	if hasAccent(last, window) || hasAccent(next, window) {
		return p.colour(reasonRedInside, s, now, urgent)
	}
	return Decision{Mode: display.ModePartial, Window: window, Reason: reasonPartial}
}

// Shown records that d reached the glass; at is the now d was decided
// with. A skip records nothing.
func (p *Policy) Shown(d Decision, at time.Time) {
	switch {
	case d.Skip:
		return
	case d.Mode == display.ModePartial:
		p.partials++
		return
	}
	p.lastColour, p.partials = at, 0
	if d.Mode == display.ModeWash {
		p.lastWash = p.washNight(at)
	}
}

// Failed records that a Show failed: the glass is unknown, the next frame
// is a full one, and the partial updates counted so far no longer
// describe anything on the glass.
func (p *Policy) Failed() {
	p.partials = 0
}

// Tick is the render cadence at now: the quiet tick inside quiet hours,
// the day tick otherwise.
func (p *Policy) Tick(now time.Time) time.Duration {
	if p.quiet(now) {
		return p.cfg.Quiet.Tick
	}
	return p.cfg.Tick
}

// colour is a colour refresh for reason, or a skip when the last one is too
// recent and the render is not urgent.
func (p *Policy) colour(reason string, s display.Surface, now time.Time, urgent bool) Decision {
	if !urgent && p.tooSoon(s, now) {
		return Decision{Skip: true, Reason: reasonTooSoon}
	}
	switch {
	case urgent:
		return Decision{Mode: display.ModeFull, Reason: reason + ", " + reasonUrgent}
	case p.quiet(now):
		return Decision{Mode: display.ModeFull, Reason: reason + ", " + reasonQuietColor}
	case !p.cfg.Fast:
		return Decision{Mode: display.ModeFull, Reason: reason}
	}
	return Decision{Mode: display.ModeFast, Reason: reason}
}

// tooSoon reports whether a colour refresh now would follow the last one
// closer than MinFullInterval. The spacing protects a panel from refreshing
// too often; a display whose refresh takes no time (a file, a web page)
// has nothing to protect and never waits.
func (p *Policy) tooSoon(s display.Surface, now time.Time) bool {
	return s.FullRefresh > 0 && !p.lastColour.IsZero() && now.Sub(p.lastColour) < p.cfg.MinFullInterval
}

// quiet reports whether now lies in the quiet hours.
func (p *Policy) quiet(now time.Time) bool {
	q := p.cfg.Quiet
	return q.enabled() && within(timeOfDay(now), q.From, q.To)
}

// washDue reports whether the nightly wash is due at now: inside [Wash, To)
// and not yet done this night.
func (p *Policy) washDue(now time.Time) bool {
	q := p.cfg.Quiet
	if !q.enabled() || !within(timeOfDay(now), q.Wash, q.To) {
		return false
	}
	return !p.washNight(now).Equal(p.lastWash)
}

// washNight names the night now belongs to by the calendar date of its
// wash moment: before Wash on the wall clock, that moment was yesterday.
// The date is kept as UTC midnight, a plain key.
func (p *Policy) washNight(now time.Time) time.Time {
	y, m, d := now.Date()
	night := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if timeOfDay(now) < p.cfg.Quiet.Wash {
		night = night.AddDate(0, 0, -1)
	}
	return night
}

// enabled reports whether there are quiet hours at all.
func (q Quiet) enabled() bool {
	return q.From != q.To
}

// validate checks the window, the tick and the wash time.
func (q Quiet) validate() error {
	if !q.enabled() {
		return nil
	}
	for _, t := range []time.Duration{q.From, q.To, q.Wash} {
		if t < 0 || t >= day {
			return fmt.Errorf("time of day %v out of range", t)
		}
	}
	if q.Tick <= 0 {
		return errors.New("tick must be positive")
	}
	if !within(q.Wash, q.From, q.To) {
		return fmt.Errorf("wash %v outside the quiet hours %v–%v", q.Wash, q.From, q.To)
	}
	return nil
}

// timeOfDay is the wall-clock offset of t from its midnight.
func timeOfDay(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
}

// within reports whether the time of day t lies in [from, to), a window
// that may cross midnight.
func within(t, from, to time.Duration) bool {
	if from <= to {
		return from <= t && t < to
	}
	return t >= from || t < to
}

// compare returns the bounding box of the pixels whose colour differs
// between two frames of the same bounds, and whether the accent colour is
// among the changes.
func compare(a, b *image.Paletted) (box image.Rectangle, red bool) {
	pa, pb := palette(a.Palette), palette(b.Palette)
	minX, minY, maxX, maxY := b.Rect.Max.X, b.Rect.Max.Y, b.Rect.Min.X-1, b.Rect.Min.Y-1
	for y := b.Rect.Min.Y; y < b.Rect.Max.Y; y++ {
		ia, ib := a.PixOffset(b.Rect.Min.X, y), b.PixOffset(b.Rect.Min.X, y)
		for x := b.Rect.Min.X; x < b.Rect.Max.X; x++ {
			ca, cb := at(pa, a.Pix[ia]), at(pb, b.Pix[ib])
			ia++
			ib++
			if ca == cb {
				continue
			}
			red = red || ca == accent || cb == accent
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	if maxX < minX {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), red
}

// hasAccent reports whether any pixel of img inside r is the accent colour.
func hasAccent(img *image.Paletted, r image.Rectangle) bool {
	p := palette(img.Palette)
	r = r.Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := img.PixOffset(r.Min.X, y)
		for range r.Dx() {
			if at(p, img.Pix[i]) == accent {
				return true
			}
			i++
		}
	}
	return false
}

// align grows r horizontally so its edges fall on multiples of the
// surface's window alignment, counted from the surface's left edge.
func align(r image.Rectangle, s display.Surface) image.Rectangle {
	a := s.WindowAlign
	if a <= 1 {
		return r
	}
	left := s.Bounds.Min.X
	r.Min.X = left + (r.Min.X-left)/a*a
	r.Max.X = left + (r.Max.X-left+a-1)/a*a
	return r
}

// palette converts a palette to comparable values.
func palette(p color.Palette) []color.RGBA {
	out := make([]color.RGBA, len(p))
	for i, c := range p {
		out[i] = rgba(c)
	}
	return out
}

// at is the colour of a palette index; an index past the palette reads as
// transparent black, which matches nothing real.
func at(p []color.RGBA, i uint8) color.RGBA {
	if int(i) < len(p) {
		return p[i]
	}
	return color.RGBA{}
}

// rgba converts any colour to a comparable value.
func rgba(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}
