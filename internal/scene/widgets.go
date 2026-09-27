package scene

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/icon"
	"github.com/mshaulsky/domovoi/internal/model"
)

// Widget draws one part of a scene into a rectangle.
type Widget interface {
	Draw(c *Canvas, r image.Rectangle, v View)
}

// Header shows the date, the freshness stamp and every source's last
// successful poll; a stale source is flagged in accent.
type Header struct{}

// AlertBar shows the first active alert and how many more there are.
type AlertBar struct{}

// ClimateTile shows one device with a temperature: its icon and name, the
// temperature large, humidity, today's extremes, trend and battery.
type ClimateTile struct {
	Device model.Device
}

// OutsideTile shows the weather device: condition, temperature, humidity,
// precipitation, forecast, wind, sunrise and sunset.
type OutsideTile struct {
	Device model.Device
}

// Hallway lists the devices with a lock, power or leak state, wrapping
// onto more rows when the strip is narrow.
type Hallway struct {
	Devices []model.Device
}

// Footer shows the advisor's note when there is one, otherwise the latest
// journal events. While starting it shows nothing: the overview says so in
// the middle of the frame.
type Footer struct{}

// scale converts design pixels to surface pixels. Strips scale with the
// surface height (design 480); a tile's contents scale with the tile, so a
// short tile stays inside its border and a tall one fills it.
type scale struct {
	factor float64
}

// Design sizes, in design pixels.
const (
	designHeight = 480
	headerHeight = 44
	alertHeight  = 40
	footerHeight = 40
	hallwayRow   = 84
	hallwayCell  = 190 // design cell width; the strip is one row, narrower cells shrink
	tileGap      = 8
	padding      = 10
	lineWidth    = 1
	footerEvents = 2 // how many journal events the footer lists
	lowBattery   = 20
	tileInnerH   = 150 // a tile's inner height at design size
	tileInnerW   = 240 // a tile's inner width at design size
	// hallwayMinScale bounds how far a crowded strip shrinks its cells.
	hallwayMinScale = 0.55
	tileMinFactor   = 0.4
	tileMaxFactor   = 1.6
)

func newScale(r image.Rectangle) scale {
	return scale{factor: float64(r.Dy()) / designHeight}
}

// tileScale fits a tile's design to its inner rectangle.
func tileScale(inner image.Rectangle) scale {
	return fitScale(inner, tileInnerW, tileInnerH)
}

// fitScale fits a design of designW×designH to a rectangle, by the tighter
// of width and height, within the clamp.
func fitScale(r image.Rectangle, designW, designH int) scale {
	f := min(float64(r.Dx())/float64(designW), float64(r.Dy())/float64(designH))
	return scale{factor: min(max(f, tileMinFactor), tileMaxFactor)}
}

// px converts a design size, never below one pixel.
func (s scale) px(n int) int {
	return max(int(float64(n)*s.factor+0.5), 1)
}

// Draw renders the header: date and freshness stamp on the left, sources
// on the right, the stamp clipped before it can run into them.
func (Header) Draw(c *Canvas, r image.Rectangle, v View) {
	s := newScale(c.Bounds())
	th := c.Theme()
	face := c.Faces().Text(s.px(20))
	iconSize := s.px(22)
	r = r.Inset(s.px(padding))
	gap := s.px(6)
	iconY := r.Min.Y + (r.Dy()-iconSize)/2

	// Right: sources, laid out right to left.
	right := r.Max.X
	for _, src := range slices.Backward(v.Sources) {
		col := th.Ink
		stamp := "—"
		if !src.LastOK.IsZero() {
			stamp = v.Clock(src.LastOK)
		}
		if src.Stale {
			col = th.Accent
		}
		label := capitalise(src.Name) + " " + stamp
		right -= c.TextWidth(label, face)
		c.Text(label, face, col, right, baseline(face, r))
		if src.Stale {
			right -= iconSize + gap/2
			c.Icon(icon.CloudOff, iconSize, col, right, iconY)
		}
		right -= s.px(16)
	}

	// Left: date, then the stamp, inside what the sources left.
	left := image.Rect(r.Min.X, r.Min.Y, right, r.Max.Y)
	x := left.Min.X
	c.Icon(icon.Calendar, iconSize, th.Ink, x, iconY)
	x += iconSize + gap
	x += c.Text(v.Bundle.Date(v.Now.In(loc(v))), face, th.Ink, x, baseline(face, left)) + s.px(24)
	stampIcon, stamp := icon.Update, v.Bundle.T("scene.updated", i18n.Args{"Time": v.Clock(v.Now)})
	if v.Restored {
		stampIcon, stamp = icon.History, v.Bundle.T("scene.restored")
	}
	c.Icon(stampIcon, iconSize, th.Ink, x, iconY)
	x += iconSize + gap
	c.TextIn(stamp, face, th.Ink, image.Rect(x, left.Min.Y, left.Max.X, left.Max.Y), AlignLeft)
}

// Draw renders the alert bar: accent background, paper text.
func (AlertBar) Draw(c *Canvas, r image.Rectangle, v View) {
	if len(v.Alerts) == 0 {
		return
	}
	s := newScale(c.Bounds())
	th := c.Theme()
	c.Fill(r, th.Accent)
	a := v.Alerts[0]
	inner := r.Inset(s.px(8))
	iconSize := s.px(26)
	ic := icon.Bell
	if a.Severity == SeverityUrgent {
		ic = icon.Alert
	}
	c.Icon(ic, iconSize, th.Paper, inner.Min.X, inner.Min.Y+(inner.Dy()-iconSize)/2)
	text := v.Bundle.T("alert."+a.Severity) + " · " + a.Message
	if !a.Since.IsZero() {
		text += " · " + v.Bundle.T("scene.since", i18n.Args{"Time": v.Clock(a.Since)})
	}
	if extra := len(v.Alerts) - 1; extra > 0 {
		text += " +" + v.Format(float64(extra), 0)
	}
	box := image.Rect(inner.Min.X+iconSize+s.px(8), inner.Min.Y, inner.Max.X, inner.Max.Y)
	face := c.FitText(text, true, s.px(20), s.px(14), box.Dx())
	c.TextIn(text, face, th.Paper, box, AlignLeft)
}

// Draw renders a climate tile.
func (t ClimateTile) Draw(c *Canvas, r image.Rectangle, v View) {
	sc := newScale(c.Bounds())
	th := c.Theme()
	c.Outline(r, sc.px(lineWidth), th.Ink)
	inner := r.Inset(sc.px(padding))
	s := tileScale(inner)
	id := t.Device.ID

	top, rest := TakeTop(inner, s.px(26))
	drawTitle(c, top, s, v.Icon(t.Device), v.DeviceName(t.Device), statusIcons(v, id))

	big, rest := TakeTop(rest, s.px(62))
	if temp, ok := v.Number(id, model.Temperature); ok {
		drawBigValue(c, big, s, icon.Thermometer, v.Format(temp, 1)+"°", v.Trend(id, model.Temperature))
	}

	line, rest := TakeTop(rest, s.px(26))
	var details []detail
	if hum, ok := v.Number(id, model.Humidity); ok {
		details = append(details, detail{icon: icon.Humidity, col: th.Ink, text: v.Format(hum, 0) + " %"})
	}
	if target, ok := v.Number(id, model.TargetTemperature); ok {
		details = append(details, detail{icon: icon.Thermostat, col: th.Ink, text: v.Format(target, 1) + "°"})
	}
	if d, ok := heatingDetail(v, id, th); ok {
		details = append(details, d)
	}
	x := line.Min.X
	for _, d := range details {
		x = drawDetail(c, line, s, x, d)
	}
	if bat, ok := v.Number(id, model.Battery); ok {
		iconSize := s.px(22)
		col := th.Ink
		if bat < lowBattery {
			col = th.Accent
		}
		c.Icon(icon.ForBattery(int(bat+0.5)), iconSize, col, line.Max.X-iconSize, line.Min.Y+(line.Dy()-iconSize)/2)
	}

	if rest.Dy() < s.px(14) {
		return
	}
	if seen, ok := v.Seen[id]; ok && v.Stale[id] {
		drawSilence(c, rest, s, v, seen)
		return
	}
	if ex, ok := v.Extremes[id][model.Temperature]; ok && ex.Min != ex.Max { // one point is no range yet
		drawExtremes(c, rest, s, v, ex, v.Readings[id][model.Temperature].Value.Num)
	}
}

// drawSilence notes when a stale device last spoke, in the accent colour,
// where a live tile shows its extremes.
func drawSilence(c *Canvas, r image.Rectangle, s scale, v View, seen time.Time) {
	face := c.Faces().Text(s.px(15))
	c.TextIn(v.Bundle.T("scene.no_data_since", i18n.Args{"Time": v.When(seen)}), face, c.Theme().Accent, r, AlignLeft)
}

// Draw renders the weather tile.
func (t OutsideTile) Draw(c *Canvas, r image.Rectangle, v View) {
	sc := newScale(c.Bounds())
	th := c.Theme()
	c.Outline(r, sc.px(lineWidth), th.Ink)
	inner := r.Inset(sc.px(padding))
	s := tileScale(inner)
	id := t.Device.ID

	top, rest := TakeTop(inner, s.px(26))
	drawTitle(c, top, s, icon.Location, v.Bundle.T("scene.outside"), statusIcons(v, id))

	code, _ := v.Number(id, model.WeatherCode)
	condition := model.WeatherConditionOf(int(code))
	big, rest := TakeTop(rest, s.px(56))
	if temp, ok := v.Number(id, model.Temperature); ok {
		drawBigValue(c, big, s, icon.ForCondition(condition), v.Format(temp, 1)+"°", icon.None)
	}

	lines := Even(rest, 3, true)
	face := c.Faces().Text(s.px(17))
	small := s.px(18)
	gap := s.px(8)
	row := lineWriter{c: c, s: s, face: face, small: small, gap: gap}

	row.start(lines[0])
	row.text(v.Bundle.T("weather." + string(condition)))
	if hum, ok := v.Number(id, model.Humidity); ok {
		row.iconText(icon.Humidity, v.Format(hum, 0)+" %")
	}
	if p, ok := v.Number(id, model.PrecipitationProbability); ok {
		row.iconText(icon.WeatherRainy, v.Bundle.T("scene.precipitation", i18n.Args{"Percent": v.Format(p, 0)}))
	}

	row.start(lines[1])
	if hi, ok := v.Number(id, model.ForecastHigh); ok {
		row.iconText(icon.ArrowUp, v.Format(hi, 0)+"°")
	}
	if lo, ok := v.Number(id, model.ForecastLow); ok {
		row.iconText(icon.ArrowDown, v.Format(lo, 0)+"°")
	}
	if wind, ok := v.Number(id, model.WindSpeed); ok {
		row.iconText(icon.WeatherWindy, v.Bundle.T("scene.wind", i18n.Args{"Speed": v.Format(wind, 0)}))
	}

	row.start(lines[2])
	if rise, ok := v.Text(id, model.Sunrise); ok {
		row.iconText(icon.Sunrise, rise)
	}
	if set, ok := v.Text(id, model.Sunset); ok {
		row.iconText(icon.Sunset, set)
	}
}

// Draw renders the hallway strip.
func (h Hallway) Draw(c *Canvas, r image.Rectangle, v View) {
	if len(h.Devices) == 0 {
		return
	}
	s := newScale(c.Bounds())
	th := c.Theme()
	cells := Grid(r, len(h.Devices), 1, 0) // one row: the strip shrinks, it never wraps
	for i, d := range h.Devices {
		cell := cells[i].Inset(s.px(padding))
		cs := fitScale(cell, hallwayCell-2*padding, hallwayRow-2*padding)
		iconSize := cs.px(40)
		ic, state, alarm := hallwayState(v, d)
		col := th.Ink
		if alarm {
			col = th.Accent
		}
		switch {
		case v.Offline(d.ID):
			ic, col, state = icon.WifiOff, th.Accent, v.Bundle.T("scene.offline")
		case v.Stale[d.ID]:
			ic, col, state = icon.CloudOff, th.Accent, v.Bundle.T("scene.stale")
		}
		c.Icon(ic, iconSize, col, cell.Min.X, cell.Min.Y+(cell.Dy()-iconSize)/2)
		text := image.Rect(cell.Min.X+iconSize+cs.px(8), cell.Min.Y, cell.Max.X, cell.Max.Y)
		name, rest := TakeTop(text, text.Dy()/2)
		label := v.DeviceName(d)
		c.TextIn(label, c.FitText(label, false, cs.px(16), cs.px(11), name.Dx()), th.Ink, name, AlignLeft)
		c.TextIn(state, c.FitText(state, true, cs.px(20), cs.px(12), rest.Dx()), col, rest, AlignLeft)
	}
}

// Draw renders the footer.
func (Footer) Draw(c *Canvas, r image.Rectangle, v View) {
	s := newScale(c.Bounds())
	th := c.Theme()
	inner := r.Inset(s.px(padding))
	iconSize := s.px(22)
	ic := icon.History
	var text string
	switch {
	case v.Starting:
		return
	case v.Note != "":
		ic, text = icon.Info, v.Note
	default:
		var parts []string
		for i, e := range v.Events {
			if i == footerEvents {
				break
			}
			parts = append(parts, v.Clock(e.At)+" "+v.Bundle.T("event."+string(e.Kind), i18n.Args{"Subject": v.eventSubject(e)}))
		}
		text = strings.Join(parts, "  ·  ")
	}
	if text == "" {
		return
	}
	c.Icon(ic, iconSize, th.Ink, inner.Min.X, inner.Min.Y+(inner.Dy()-iconSize)/2)
	box := image.Rect(inner.Min.X+iconSize+s.px(8), inner.Min.Y, inner.Max.X, inner.Max.Y)
	c.TextIn(text, c.FitText(text, false, s.px(18), s.px(13), box.Dx()), th.Ink, box, AlignLeft)
}

// lineWriter lays icon+text pairs along one line, left to right.
type lineWriter struct {
	c     *Canvas
	s     scale
	face  font.Face
	small int
	gap   int
	line  image.Rectangle
	x     int
}

func (w *lineWriter) start(line image.Rectangle) {
	w.line = line
	w.x = line.Min.X
}

func (w *lineWriter) text(s string) {
	w.c.stampIn(w.face, s, w.c.Theme().Ink, fixed.P(w.x, baseline(w.face, w.line)), w.line)
	w.x += w.c.TextWidth(s, w.face) + w.gap
}

func (w *lineWriter) iconText(ic icon.Icon, s string) {
	if w.x+w.small > w.line.Max.X {
		return
	}
	w.c.Icon(ic, w.small, w.c.Theme().Ink, w.x, w.line.Min.Y+(w.line.Dy()-w.small)/2)
	w.x += w.small + w.s.px(2)
	w.text(s)
}

// eventSubject names what an event is about: the device, or the detail
// for device-less events such as alerts.
func (v View) eventSubject(e model.Event) string {
	for _, d := range v.Devices {
		if d.ID == e.Device {
			return v.DeviceName(d)
		}
	}
	if e.Detail != "" {
		return e.Detail
	}
	return string(e.Device)
}

// drawTitle draws a tile's icon and name, with status icons at the right.
func drawTitle(c *Canvas, r image.Rectangle, s scale, ic icon.Icon, name string, status []icon.Icon) {
	th := c.Theme()
	iconSize := s.px(24)
	c.Icon(ic, iconSize, th.Ink, r.Min.X, r.Min.Y+(r.Dy()-iconSize)/2)
	x := r.Max.X
	for _, st := range status {
		x -= iconSize
		c.Icon(st, iconSize, th.Accent, x, r.Min.Y+(r.Dy()-iconSize)/2)
		x -= s.px(4)
	}
	text := image.Rect(r.Min.X+iconSize+s.px(6), r.Min.Y, x, r.Max.Y)
	c.TextIn(name, c.FitText(name, true, s.px(20), s.px(12), text.Dx()), th.Ink, text, AlignLeft)
}

// drawBigValue draws the tile's headline: an icon, a large value and an
// optional trend arrow after it. The value shrinks to fit the width.
func drawBigValue(c *Canvas, r image.Rectangle, s scale, ic icon.Icon, value string, trend icon.Icon) {
	th := c.Theme()
	iconSize := s.px(44)
	trendSize := 0
	if trend != icon.None {
		trendSize = s.px(28) + s.px(4)
	}
	c.Icon(ic, iconSize, th.Ink, r.Min.X, r.Min.Y+(r.Dy()-iconSize)/2)
	x := r.Min.X + iconSize + s.px(6)
	face := c.FitText(value, true, s.px(52), s.px(24), r.Max.X-x-trendSize)
	x += c.Text(value, face, th.Ink, x, baseline(face, r))
	if trend != icon.None {
		t := s.px(28)
		c.Icon(trend, t, th.Ink, x+s.px(4), r.Min.Y+(r.Dy()-t)/2)
	}
}

// drawExtremes draws today's minimum and maximum; the one the current value
// equals is accented, because that is a record being set right now.
func drawExtremes(c *Canvas, r image.Rectangle, s scale, v View, ex Extremes, current float64) {
	th := c.Theme()
	face := c.Faces().Text(s.px(15))
	iconSize := s.px(16)
	x := r.Min.X
	for _, e := range []struct {
		ic  icon.Icon
		val float64
	}{{icon.ArrowDown, ex.Min}, {icon.ArrowUp, ex.Max}} {
		col := th.Ink
		if current == e.val {
			col = th.Accent
		}
		c.Icon(e.ic, iconSize, col, x, r.Min.Y+(r.Dy()-iconSize)/2)
		x += iconSize
		x += c.Text(v.Format(e.val, 1)+"°", face, col, x, baseline(face, r)) + s.px(10)
	}
}

// statusIcons lists the accent icons a tile shows in its corner.
func statusIcons(v View, id model.DeviceID) []icon.Icon {
	var out []icon.Icon
	if v.Stale[id] {
		out = append(out, icon.CloudOff)
	}
	if v.Offline(id) {
		out = append(out, icon.WifiOff)
	}
	return out
}

// detail is one icon-and-text item on a tile's detail line.
type detail struct {
	icon icon.Icon
	col  color.Color
	text string
}

// drawDetail draws one detail at x and returns where the next one starts.
func drawDetail(c *Canvas, line image.Rectangle, s scale, x int, d detail) int {
	face := c.Faces().Text(s.px(19))
	iconSize := s.px(20)
	c.Icon(d.icon, iconSize, d.col, x, line.Min.Y+(line.Dy()-iconSize)/2)
	x += iconSize + s.px(4)
	return x + c.Text(d.text, face, d.col, x, baseline(face, line)) + s.px(10)
}

// heatingDetail describes a thermostat: switched off, on, or heating right
// now (its valve open), which is drawn in the accent colour.
func heatingDetail(v View, id model.DeviceID, th Theme) (detail, bool) {
	on, hasPower := v.Bool(id, model.Power)
	heating, hasHeating := v.Bool(id, model.Heating)
	switch {
	case !hasPower && !hasHeating:
		return detail{}, false
	case hasPower && !on:
		return detail{icon: icon.Radiator, col: th.Ink, text: v.Bundle.T("scene.off")}, true
	case hasHeating && heating:
		return detail{icon: icon.Radiator, col: th.Accent, text: v.Bundle.T("scene.heating")}, true
	default:
		return detail{icon: icon.Radiator, col: th.Ink, text: v.Bundle.T("scene.on")}, true
	}
}

// hallwayHeight is the strip height for n devices in one row: the design
// row height while the cells are at least hallwayCell wide, shrinking with
// the cells below that, never under hallwayMinScale of the design.
func hallwayHeight(width int, s scale, n int) int {
	ratio := float64(width/max(n, 1)) / float64(s.px(hallwayCell))
	ratio = min(1, max(hallwayMinScale, ratio))
	return int(float64(s.px(hallwayRow))*ratio + 0.5)
}

// hallwayState picks the icon and state word of a hallway device, and
// whether the state is alarming.
func hallwayState(v View, d model.Device) (ic icon.Icon, state string, alarm bool) {
	if locked, ok := v.Bool(d.ID, model.Locked); ok {
		if locked {
			return icon.Lock, v.Bundle.T("scene.locked"), false
		}
		return icon.LockOpen, v.Bundle.T("scene.unlocked"), true
	}
	if leak, ok := v.Bool(d.ID, model.Leak); ok {
		if leak {
			return icon.Water, v.Bundle.T("scene.leak"), true
		}
		return icon.WaterOff, v.Bundle.T("scene.dry"), false
	}
	if on, ok := v.Bool(d.ID, model.Power); ok {
		if !on {
			return icon.PowerPlugOff, v.Bundle.T("scene.off"), false
		}
		state := v.Bundle.T("scene.on")
		if w, ok := v.Number(d.ID, model.PowerDraw); ok {
			state += " · " + v.Bundle.T("scene.watts", i18n.Args{"Value": v.Format(w, 0)})
		}
		return v.Icon(d), state, false
	}
	return v.Icon(d), "", false
}

func loc(v View) *time.Location {
	if v.Location != nil {
		return v.Location
	}
	return time.UTC
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
