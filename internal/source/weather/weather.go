package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
)

// Source polls Open-Meteo for one location.
type Source struct {
	cfg     Config
	http    Doer
	log     *slog.Logger
	metrics Metrics
}

// Config is what the source needs.
type Config struct {
	Name      string         // source name, prefix of its device ID
	Latitude  float64        // degrees, -90..90
	Longitude float64        // degrees, -180..180
	Location  *time.Location // the day and the sun times are local to it; defaults to UTC
	BaseURL   string         // the forecast endpoint; defaults to DefaultBaseURL
}

// response is the part of Open-Meteo's answer the source reads. Pointers
// because the API answers null for a value it does not have.
type response struct {
	UTCOffsetSeconds *int `json:"utc_offset_seconds"` // the zone the API wrote its times in
	Current          struct {
		Time        string   `json:"time"`
		Temperature *float64 `json:"temperature_2m"`
		Humidity    *float64 `json:"relative_humidity_2m"`
		WeatherCode *float64 `json:"weather_code"`
		WindSpeed   *float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Daily struct {
		High          []*float64 `json:"temperature_2m_max"`
		Low           []*float64 `json:"temperature_2m_min"`
		Sunrise       []*string  `json:"sunrise"`
		Sunset        []*string  `json:"sunset"`
		Precipitation []*float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

const (
	// DefaultBaseURL is Open-Meteo's forecast endpoint.
	DefaultBaseURL = "https://api.open-meteo.com/v1/forecast"
	// native is the device's ID within the source: the device is <name>:home.
	native = "home"
	// isoMinute is how Open-Meteo writes local times with timeformat=iso8601.
	isoMinute = "2006-01-02T15:04"
	// currentFields and dailyFields are the variables requested.
	currentFields = "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m"
	dailyFields   = "temperature_2m_max,temperature_2m_min,sunrise,sunset,precipitation_probability_max"
	// maxBody bounds how much of an answer is read; a forecast is a few KB.
	maxBody = 1 << 20
)

// New validates the config and wires the source to an HTTP client.
func New(cfg Config, client Doer, log *slog.Logger, m Metrics) (*Source, error) {
	if cfg.Name == "" {
		return nil, errors.New("weather: source name is required")
	}
	if client == nil {
		return nil, errors.New("weather: http client is required")
	}
	if cfg.Latitude < -90 || cfg.Latitude > 90 {
		return nil, fmt.Errorf("weather: latitude %g out of range -90..90", cfg.Latitude)
	}
	if cfg.Longitude < -180 || cfg.Longitude > 180 {
		return nil, fmt.Errorf("weather: longitude %g out of range -180..180", cfg.Longitude)
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if u, err := url.Parse(cfg.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("weather: base URL %q is not an absolute URL", cfg.BaseURL)
	}
	return &Source{
		cfg:     cfg,
		http:    client,
		log:     log.With("component", "source.weather", "source", cfg.Name),
		metrics: m,
	}, nil
}

// Name returns the configured source name.
func (s *Source) Name() string {
	return s.cfg.Name
}

// Poll fetches the forecast once. Batch.Devices carries the virtual device
// on the first successful poll and is nil afterwards. Readings are stamped
// with the model's time for the current conditions, clamped to the poll
// time; the poll time when the answer has none.
func (s *Source) Poll(ctx context.Context) (source.Batch, error) {
	now := time.Now()
	req, err := s.request(ctx)
	if err != nil {
		return source.Batch{}, err
	}
	s.metrics.IncRequest(s.cfg.Name)
	resp, err := s.http.Do(req)
	if err != nil {
		return source.Batch{}, fmt.Errorf("weather: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return source.Batch{}, fmt.Errorf("weather: fetch: %s", status(resp))
	}
	var r response
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&r); err != nil {
		return source.Batch{}, fmt.Errorf("weather: decode: %w", err)
	}

	// The device rides in every batch: it is one row, and a batch the store
	// had to skip would otherwise leave every later write without its
	// device row.
	id := model.NewDeviceID(s.cfg.Name, native)
	batch := source.Batch{Devices: []model.Device{{ID: id, Name: native, Kind: model.KindVirtual}}}
	batch.Readings = s.readings(id, r, s.stamp(r, now))
	s.log.Debug("forecast fetched", "at", r.Current.Time, "readings", len(batch.Readings))
	return batch, nil
}

// timezone is what the API takes: an IANA name, or "auto" — resolved from
// the coordinates — for the system zone, whose name is the literal "Local"
// the API would reject.
func (s *Source) timezone() string {
	if name := s.cfg.Location.String(); name != "Local" {
		return name
	}
	return "auto"
}

// request builds the forecast query.
func (s *Source) request(ctx context.Context) (*http.Request, error) {
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(s.cfg.Latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(s.cfg.Longitude, 'f', -1, 64))
	q.Set("current", currentFields)
	q.Set("daily", dailyFields)
	q.Set("timezone", s.timezone())
	q.Set("forecast_days", "1")
	q.Set("wind_speed_unit", "ms")
	q.Set("timeformat", "iso8601")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BaseURL+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("weather: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "domovoi")
	return req, nil
}

// readings maps the answer into the model; a value the API has no answer
// for yields no reading.
func (s *Source) readings(id model.DeviceID, r response, at time.Time) []model.Reading {
	var out []model.Reading
	number := func(m model.Metric, v *float64) {
		if v != nil {
			out = append(out, model.Reading{Device: id, Metric: m, Value: model.NumberValue(*v), At: at})
		}
	}
	clock := func(m model.Metric, v *string) {
		if v == nil {
			return
		}
		t, err := time.ParseInLocation(isoMinute, *v, s.cfg.Location)
		if err != nil {
			s.log.Debug("sun time unreadable, dropped", "metric", m, "value", *v, "err", err)
			return
		}
		out = append(out, model.Reading{Device: id, Metric: m, Value: model.TextValue(t.Format("15:04")), At: at})
	}
	number(model.Temperature, r.Current.Temperature)
	number(model.Humidity, r.Current.Humidity)
	number(model.WeatherCode, r.Current.WeatherCode)
	number(model.WindSpeed, r.Current.WindSpeed)
	number(model.PrecipitationProbability, first(r.Daily.Precipitation))
	number(model.ForecastHigh, first(r.Daily.High))
	number(model.ForecastLow, first(r.Daily.Low))
	clock(model.Sunrise, first(r.Daily.Sunrise))
	clock(model.Sunset, first(r.Daily.Sunset))
	out = append(out, model.Reading{Device: id, Metric: model.Online, Value: model.BoolValue(true), At: at})
	return out
}

// stamp parses the model's time for the current conditions, in the zone the
// answer reports; the poll time when it is missing, unreadable or ahead.
func (s *Source) stamp(r response, now time.Time) time.Time {
	// The API writes its times in the zone it was asked for — the
	// coordinates' own under "auto" — and reports that zone's offset, which
	// is what dates them correctly whatever the system zone is.
	loc := s.cfg.Location
	if r.UTCOffsetSeconds != nil {
		loc = time.FixedZone("", *r.UTCOffsetSeconds)
	}
	t, err := time.ParseInLocation(isoMinute, r.Current.Time, loc)
	if err != nil || t.After(now) {
		return now
	}
	return t
}

// status describes a non-2xx answer: the code, and Open-Meteo's reason
// when it gives one.
func status(resp *http.Response) string {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err == nil && body.Reason != "" {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, body.Reason)
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// first returns the first element of a daily array, nil when absent.
func first[T any](values []*T) *T {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}
