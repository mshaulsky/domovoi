package tuya

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/mshaulsky/tuyacloud"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
)

// Source polls a Tuya cloud project.
type Source struct {
	cfg     Config
	client  Client
	log     *slog.Logger
	metrics Metrics

	mu      sync.Mutex
	devices []tuyacloud.Device                 // the last listing; nil until the first succeeds
	polls   int                                // successful polls so far
	specs   map[string]tuyacloud.Specification // by product ID
	warned  map[string]bool                    // products whose specification failed, warned once
	retryAt map[string]time.Time               // products whose specification failed: no new attempt before
	seen    map[string]bool                    // data point codes dropped, logged once
}

// Config is what the source needs; credentials are handled by the caller,
// which builds the client.
type Config struct {
	Name      string // source name, prefix of its device IDs
	ListEvery int    // list devices every this many polls; defaults to DefaultListEvery
}

// point is how one data-point code maps into the model.
type point struct {
	metric model.Metric
	kind   model.ValueKind
	scale  int    // power-of-ten divisor for numbers while the product specification is unknown
	truth  string // for an enum standing for a boolean: the value that means true
}

const (
	// batchSize is how many devices one status request covers; the client
	// batches transparently, this only sizes the request count.
	batchSize = 20
	// DefaultListEvery relists the devices every fifth poll: with a minute
	// between polls, names, online flags and update times refresh every
	// five minutes.
	DefaultListEvery = 5
	// specRetryAfter is the hold-off before a product's failed specification
	// fetch is attempted again; numbers use the default scales meanwhile.
	specRetryAfter = 10 * time.Minute
)

// points maps Tuya data-point codes to metrics. Codes not listed are dropped.
var points = map[string]point{
	"va_temperature":     {metric: model.Temperature, kind: model.Number, scale: 1},
	"temp_current":       {metric: model.Temperature, kind: model.Number, scale: 1},
	"va_humidity":        {metric: model.Humidity, kind: model.Number},
	"humidity_value":     {metric: model.Humidity, kind: model.Number},
	"battery_percentage": {metric: model.Battery, kind: model.Number},
	"temp_set":           {metric: model.TargetTemperature, kind: model.Number, scale: 1},
	"valve_state":        {metric: model.Heating, kind: model.Bool, truth: "open"},
	"temp_alarm":         {metric: model.TemperatureAlarm, kind: model.Text},
	"hum_alarm":          {metric: model.HumidityAlarm, kind: model.Text},
	"cur_power":          {metric: model.PowerDraw, kind: model.Number, scale: 1},
	"switch":             {metric: model.Power, kind: model.Bool},
	"switch_1":           {metric: model.Power, kind: model.Bool},
}

// kinds maps Tuya product categories to device kinds.
var kinds = map[string]model.Kind{
	"wsdcg": model.KindClimateSensor,
	"wk":    model.KindThermostat,
	"cz":    model.KindOutlet,
	"pc":    model.KindOutlet,
}

// New wires the source to a client.
func New(cfg Config, client Client, log *slog.Logger, m Metrics) (*Source, error) {
	if cfg.Name == "" {
		return nil, errors.New("tuya: source name is required")
	}
	if client == nil {
		return nil, errors.New("tuya: client is required")
	}
	if cfg.ListEvery <= 0 {
		cfg.ListEvery = DefaultListEvery
	}
	return &Source{
		cfg:     cfg,
		client:  client,
		log:     log.With("component", "source.tuya", "source", cfg.Name),
		metrics: m,
		specs:   map[string]tuyacloud.Specification{},
		warned:  map[string]bool{},
		retryAt: map[string]time.Time{},
		seen:    map[string]bool{},
	}, nil
}

// Name returns the configured source name.
func (s *Source) Name() string {
	return s.cfg.Name
}

// Poll reads the devices' status, listing the devices first on the first
// poll, after a failure, and every ListEvery-th poll. Batch.Devices is nil
// on the polls in between: unchanged. Offline devices are read too: the
// cloud answers with their last known values, which the tile shows next to
// the offline mark. Readings are stamped with the device's update time from
// the listing, not with the poll time, so cached values carry their real
// age.
func (s *Source) Poll(ctx context.Context) (source.Batch, error) {
	now := time.Now()
	var batch source.Batch
	devices, listed, err := s.listing(ctx)
	if err != nil {
		return source.Batch{}, err
	}
	if listed {
		batch.Devices = make([]model.Device, 0, len(devices))
		for _, d := range devices {
			id := model.NewDeviceID(s.cfg.Name, d.ID)
			batch.Devices = append(batch.Devices, model.Device{ID: id, Name: d.Name, Kind: kindOf(d.Category)})
			batch.Readings = append(batch.Readings, model.Reading{Device: id, Metric: model.Online, Value: model.BoolValue(d.Online), At: stamp(d, now)})
		}
	}
	ids := make([]string, 0, len(devices))
	for _, d := range devices {
		ids = append(ids, d.ID)
	}
	if len(ids) == 0 {
		s.polled()
		return batch, nil
	}
	for range (len(ids) + batchSize - 1) / batchSize {
		s.metrics.IncRequest(s.cfg.Name)
	}
	statuses, err := s.client.DevicesStatus(ctx, ids...)
	if err != nil {
		return source.Batch{}, fmt.Errorf("tuya: read status: %w", err)
	}
	for _, d := range devices {
		status, ok := statuses[d.ID]
		if !ok {
			continue
		}
		spec, hasSpec := s.specification(ctx, d)
		id := model.NewDeviceID(s.cfg.Name, d.ID)
		at := stamp(d, now)
		for _, dp := range status {
			r, ok := s.convert(id, dp, spec, hasSpec, at)
			if ok {
				batch.Readings = append(batch.Readings, r)
			}
		}
	}
	s.polled()
	return batch, nil
}

// listing returns the devices to read: a fresh listing when due, the
// cached one otherwise.
func (s *Source) listing(ctx context.Context) (devices []tuyacloud.Device, listed bool, err error) {
	s.mu.Lock()
	cached, due := s.devices, s.devices == nil || s.polls%s.cfg.ListEvery == 0
	s.mu.Unlock()
	if !due {
		return cached, false, nil
	}
	s.metrics.IncRequest(s.cfg.Name)
	devices, err = s.client.Devices(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("tuya: list devices: %w", err)
	}
	if devices == nil {
		devices = []tuyacloud.Device{} // an empty project is still a listing
	}
	s.mu.Lock()
	s.devices = devices
	s.mu.Unlock()
	return devices, true, nil
}

// polled counts a successful pass.
func (s *Source) polled() {
	s.mu.Lock()
	s.polls++
	s.mu.Unlock()
}

// specification returns the product's specification, fetching and caching
// it on first sight. A failed fetch is warned once and not retried before
// specRetryAfter; meanwhile numbers use the default scales of the points
// table. The lock is not held across the network call.
func (s *Source) specification(ctx context.Context, d tuyacloud.Device) (tuyacloud.Specification, bool) {
	s.mu.Lock()
	spec, ok := s.specs[d.ProductID]
	retryAt := s.retryAt[d.ProductID]
	s.mu.Unlock()
	if ok {
		return spec, true
	}
	if time.Now().Before(retryAt) {
		return tuyacloud.Specification{}, false
	}
	s.metrics.IncRequest(s.cfg.Name)
	spec, err := s.client.Specification(ctx, d.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.retryAt[d.ProductID] = time.Now().Add(specRetryAfter)
		if !s.warned[d.ProductID] {
			s.log.Warn("specification unavailable, numbers use default scales", "device", d.ID, "product", d.ProductID, "retry_in", specRetryAfter, "err", err)
			s.warned[d.ProductID] = true
		}
		return tuyacloud.Specification{}, false
	}
	s.specs[d.ProductID] = spec
	delete(s.warned, d.ProductID)
	delete(s.retryAt, d.ProductID)
	return spec, true
}

// convert maps one data point into a reading, or reports that it is dropped.
func (s *Source) convert(id model.DeviceID, dp tuyacloud.DataPoint, spec tuyacloud.Specification, hasSpec bool, at time.Time) (model.Reading, bool) {
	p, ok := points[dp.Code]
	if !ok {
		s.dropped(dp.Code)
		return model.Reading{}, false
	}
	r := model.Reading{Device: id, Metric: p.metric, At: at}
	switch p.kind {
	case model.Number:
		raw, err := dp.Int()
		if err != nil {
			s.log.Debug("data point is not an integer", "device", id, "code", dp.Code, "err", err)
			return model.Reading{}, false
		}
		if hasSpec {
			r.Value = model.NumberValue(scale(spec, dp.Code, raw))
		} else {
			r.Value = model.NumberValue(float64(raw) / math.Pow10(p.scale))
		}
	case model.Bool:
		b, err := s.truth(dp, p)
		if err != nil {
			s.log.Debug("data point is not a boolean", "device", id, "code", dp.Code, "err", err)
			return model.Reading{}, false
		}
		r.Value = model.BoolValue(b)
	case model.Text:
		t, err := dp.Text()
		if err != nil {
			s.log.Debug("data point is not a string", "device", id, "code", dp.Code, "err", err)
			return model.Reading{}, false
		}
		r.Value = model.TextValue(t)
	}
	return r, true
}

// truth reads a boolean point, or an enum point whose one value means true.
func (s *Source) truth(dp tuyacloud.DataPoint, p point) (bool, error) {
	if p.truth == "" {
		return dp.Bool()
	}
	t, err := dp.Text()
	if err != nil {
		return false, err
	}
	return t == p.truth, nil
}

// dropped logs an unmapped code once.
func (s *Source) dropped(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[code] {
		return
	}
	s.seen[code] = true
	s.log.Debug("data point maps to no metric, dropped", "code", code)
}

// scale applies the product's power-of-ten scale to a raw integer; a code
// the specification does not describe is taken at face value.
func scale(spec tuyacloud.Specification, code string, raw int64) float64 {
	def, ok := spec.Get(code)
	if !ok {
		return float64(raw)
	}
	rng, err := def.Range()
	if err != nil {
		return float64(raw)
	}
	return rng.Scaled(raw)
}

// stamp is when the device last reported, per the listing; the poll time
// when the listing has no update time or claims one from the future.
func stamp(d tuyacloud.Device, now time.Time) time.Time {
	if d.UpdateTime <= 0 {
		return now
	}
	t := time.Unix(d.UpdateTime, 0)
	if t.After(now) {
		return now
	}
	return t
}

func kindOf(category string) model.Kind {
	if k, ok := kinds[category]; ok {
		return k
	}
	return model.KindOther
}
