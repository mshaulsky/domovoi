package weather

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/domovoi/internal/model"
)

var (
	quiet  = slog.New(slog.NewTextHandler(io.Discard, nil))
	almaty = time.FixedZone("Asia/Almaty", 5*3600)
)

// forecast is a realistic Open-Meteo answer for the requested variables.
const forecast = `{
  "latitude": 43.25, "longitude": 76.9, "generationtime_ms": 0.2, "utc_offset_seconds": 18000,
  "timezone": "Asia/Almaty", "timezone_abbreviation": "+05", "elevation": 780.0,
  "current_units": {"time": "iso8601", "interval": "seconds", "temperature_2m": "°C", "relative_humidity_2m": "%", "weather_code": "wmo code", "wind_speed_10m": "m/s"},
  "current": {"time": "2026-09-27T12:45", "interval": 900, "temperature_2m": 17.3, "relative_humidity_2m": 62, "weather_code": 61, "wind_speed_10m": 4.2},
  "daily_units": {"time": "iso8601", "temperature_2m_max": "°C", "temperature_2m_min": "°C", "sunrise": "iso8601", "sunset": "iso8601", "precipitation_probability_max": "%"},
  "daily": {"time": ["2026-09-27"], "temperature_2m_max": [21.4], "temperature_2m_min": [9.1], "sunrise": ["2026-09-27T06:41"], "sunset": ["2026-09-27T19:12"], "precipitation_probability_max": [40]}
}`

// forecastWithNulls is the same answer with values the API had no data for.
const forecastWithNulls = `{
  "current": {"time": "2026-09-27T12:45", "temperature_2m": 17.3, "relative_humidity_2m": null, "weather_code": 61, "wind_speed_10m": null},
  "daily": {"time": ["2026-09-27"], "temperature_2m_max": [null], "temperature_2m_min": [9.1], "sunrise": [null], "sunset": ["2026-09-27T19:12"], "precipitation_probability_max": [null]}
}`

type values map[model.Metric]model.Value

// newSource builds a source against a test server; the server's client is
// the Doer and its URL the base.
func newSource(t *testing.T, srv *httptest.Server, cfg Config) (*Source, *MockMetrics) {
	t.Helper()
	m := NewMockMetrics(gomock.NewController(t))
	if cfg.Name == "" {
		cfg.Name = "weather"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = srv.URL + "/v1/forecast"
	}
	if cfg.Location == nil {
		cfg.Location = almaty
	}
	s, err := New(cfg, srv.Client(), quiet, m)
	if err != nil {
		t.Fatal(err)
	}
	return s, m
}

func TestNew(t *testing.T) {
	client := &http.Client{}
	tests := []struct {
		name    string
		cfg     Config
		client  Doer
		wantErr string
	}{
		{name: "valid", cfg: Config{Name: "weather", Latitude: 43.25, Longitude: 76.9, Location: almaty, BaseURL: "https://example.test/f"}, client: client},
		{name: "defaults", cfg: Config{Name: "weather"}, client: client},
		{name: "name required", cfg: Config{}, client: client, wantErr: "source name is required"},
		{name: "client required", cfg: Config{Name: "weather"}, wantErr: "http client is required"},
		{name: "latitude out of range", cfg: Config{Name: "weather", Latitude: 91}, client: client, wantErr: "latitude 91 out of range"},
		{name: "longitude out of range", cfg: Config{Name: "weather", Longitude: -181}, client: client, wantErr: "longitude -181 out of range"},
		{name: "relative base URL", cfg: Config{Name: "weather", BaseURL: "/v1/forecast"}, client: client, wantErr: "not an absolute URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.cfg, tt.client, quiet, NewMockMetrics(gomock.NewController(t)))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.cfg.BaseURL == "" && s.cfg.BaseURL != DefaultBaseURL {
				t.Errorf("BaseURL = %q, want the default", s.cfg.BaseURL)
			}
			if tt.cfg.Location == nil && s.cfg.Location != time.UTC {
				t.Errorf("Location = %v, want UTC", s.cfg.Location)
			}
		})
	}
}

func TestSourceName(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	s, _ := newSource(t, srv, Config{Name: "outside"})
	if got := s.Name(); got != "outside" {
		t.Errorf("Name() = %q", got)
	}
}

func TestSourcePoll(t *testing.T) {
	stamp := time.Date(2026, 9, 27, 12, 45, 0, 0, almaty)
	everything := values{
		model.Temperature: model.NumberValue(17.3), model.Humidity: model.NumberValue(62), model.WeatherCode: model.NumberValue(61),
		model.WindSpeed: model.NumberValue(4.2), model.PrecipitationProbability: model.NumberValue(40),
		model.ForecastHigh: model.NumberValue(21.4), model.ForecastLow: model.NumberValue(9.1),
		model.Sunrise: model.TextValue("06:41"), model.Sunset: model.TextValue("19:12"), model.Online: model.BoolValue(true),
	}
	serve := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}
	}
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		baseURL   string         // overrides the test server
		location  *time.Location // nil = Almaty
		polls     int
		wantQuery url.Values
		devices   []model.Device // what the last batch carried
		readings  values
		stamp     time.Time // zero = the poll time
		wantErr   string
	}{
		{
			name:    "first poll announces the device and reads everything",
			handler: serve(http.StatusOK, forecast),
			polls:   1,
			wantQuery: url.Values{
				"latitude": {"43.25"}, "longitude": {"76.9"},
				"current":  {"temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m"},
				"daily":    {"temperature_2m_max,temperature_2m_min,sunrise,sunset,precipitation_probability_max"},
				"timezone": {"Asia/Almaty"}, "forecast_days": {"1"}, "wind_speed_unit": {"ms"}, "timeformat": {"iso8601"},
			},
			devices:  []model.Device{{ID: "weather:home", Name: "home", Kind: model.KindVirtual}},
			readings: everything,
			stamp:    stamp,
		},
		{
			name:      "the system zone is asked for as auto",
			handler:   serve(http.StatusOK, forecast),
			polls:     1,
			location:  time.Local,
			wantQuery: url.Values{"timezone": {"auto"}},
			devices:   []model.Device{{ID: "weather:home", Name: "home", Kind: model.KindVirtual}},
			readings:  everything,
			stamp:     stamp, // dated by the offset the API reports, not the system zone
		},
		{
			name:    "every poll carries the device again",
			handler: serve(http.StatusOK, forecast),
			polls:   2,
			devices: []model.Device{{ID: "weather:home", Name: "home", Kind: model.KindVirtual}},
			readings: values{
				model.Temperature: model.NumberValue(17.3), model.Humidity: model.NumberValue(62), model.WeatherCode: model.NumberValue(61),
				model.WindSpeed: model.NumberValue(4.2), model.PrecipitationProbability: model.NumberValue(40),
				model.ForecastHigh: model.NumberValue(21.4), model.ForecastLow: model.NumberValue(9.1),
				model.Sunrise: model.TextValue("06:41"), model.Sunset: model.TextValue("19:12"), model.Online: model.BoolValue(true),
			},
			stamp: stamp,
		},
		{
			name:    "values the API has no answer for yield no reading",
			handler: serve(http.StatusOK, forecastWithNulls),
			polls:   1,
			devices: []model.Device{{ID: "weather:home", Name: "home", Kind: model.KindVirtual}},
			readings: values{
				model.Temperature: model.NumberValue(17.3), model.WeatherCode: model.NumberValue(61),
				model.ForecastLow: model.NumberValue(9.1), model.Sunset: model.TextValue("19:12"), model.Online: model.BoolValue(true),
			},
			stamp: stamp,
		},
		{
			name:     "an unreadable current time stamps with the poll time",
			handler:  serve(http.StatusOK, `{"current": {"time": "soon", "temperature_2m": 1}, "daily": {}}`),
			polls:    1,
			devices:  []model.Device{{ID: "weather:home", Name: "home", Kind: model.KindVirtual}},
			readings: values{model.Temperature: model.NumberValue(1), model.Online: model.BoolValue(true)},
		},
		{
			name:    "server error with a reason",
			handler: serve(http.StatusBadRequest, `{"error": true, "reason": "Latitude must be in range of -90 to 90°."}`),
			polls:   1,
			wantErr: "weather: fetch: HTTP 400: Latitude must be in range",
		},
		{
			name:    "server error without a body",
			handler: serve(http.StatusInternalServerError, ""),
			polls:   1,
			wantErr: "weather: fetch: HTTP 500",
		},
		{
			name:    "malformed answer",
			handler: serve(http.StatusOK, `{"current": [`),
			polls:   1,
			wantErr: "weather: decode:",
		},
		{
			name:    "unreachable endpoint",
			handler: serve(http.StatusOK, forecast),
			baseURL: "http://127.0.0.1:1/v1/forecast",
			polls:   1,
			wantErr: "weather: fetch:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				tt.handler(w, r)
			}))
			defer srv.Close()
			s, m := newSource(t, srv, Config{Latitude: 43.25, Longitude: 76.9, Location: tt.location, BaseURL: tt.baseURL})
			requests := 0
			m.EXPECT().IncRequest("weather").Do(func(string) { requests++ }).AnyTimes()

			before := time.Now()
			var (
				batch = struct {
					devices  []model.Device
					readings []model.Reading
				}{}
				err error
			)
			for range tt.polls {
				b, e := s.Poll(t.Context())
				batch.devices, batch.readings, err = b.Devices, b.Readings, e
			}
			after := time.Now()
			if requests != tt.polls {
				t.Errorf("requests counted = %d, want %d", requests, tt.polls)
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
			if tt.wantQuery != nil {
				for k, want := range tt.wantQuery {
					if got := gotQuery[k]; len(got) != 1 || got[0] != want[0] {
						t.Errorf("query %s = %v, want %v", k, got, want)
					}
				}
			}
			if len(batch.devices) != len(tt.devices) || (len(tt.devices) > 0 && batch.devices[0] != tt.devices[0]) {
				t.Errorf("devices = %v, want %v", batch.devices, tt.devices)
			}
			got := values{}
			for _, r := range batch.readings {
				if r.Device != "weather:home" {
					t.Errorf("reading for %s, want weather:home", r.Device)
				}
				got[r.Metric] = r.Value
				switch {
				case !tt.stamp.IsZero() && !r.At.Equal(tt.stamp):
					t.Errorf("%s stamped %v, want %v", r.Metric, r.At, tt.stamp)
				case tt.stamp.IsZero() && (r.At.Before(before) || r.At.After(after)):
					t.Errorf("%s stamped %v, want the poll time", r.Metric, r.At)
				}
			}
			if len(got) != len(tt.readings) {
				t.Errorf("readings = %v, want %v", got, tt.readings)
			}
			for metric, want := range tt.readings {
				if v, ok := got[metric]; !ok || !v.Equal(want) {
					t.Errorf("%s = %v, want %v", metric, v, want)
				}
			}
		})
	}
}

func TestSourcePollCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	s, m := newSource(t, srv, Config{})
	m.EXPECT().IncRequest("weather")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Poll(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Poll = %v, want context.Canceled", err)
	}
}
