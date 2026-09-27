package tuya

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/tuyacloud"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

var (
	office  = tuyacloud.Device{ID: "bf22", Name: "кабинет", Category: "wsdcg", Online: true, ProductID: "p-climate"}
	kitchen = tuyacloud.Device{ID: "bf06", Name: "Кухня", Category: "wsdcg", Online: false, ProductID: "p-climate"}
	strip   = tuyacloud.Device{ID: "bf21", Name: "Розетка", Category: "pc", Online: true, ProductID: "p-strip"}
)

var climateSpec = tuyacloud.Specification{Status: []tuyacloud.Definition{
	{Code: "va_temperature", Type: "Integer", Values: `{"unit":"℃","min":-200,"max":600,"scale":1,"step":1}`},
	{Code: "va_humidity", Type: "Integer", Values: `{"unit":"%","min":0,"max":100,"scale":0,"step":1}`},
	{Code: "battery_percentage", Type: "Integer", Values: `{"unit":"%","min":0,"max":100,"scale":0,"step":1}`},
	{Code: "temp_alarm", Type: "Enum", Values: `{"range":["loweralarm","upperalarm","cancel"]}`},
}}

// values is the readings of one batch, by device and metric.
type values map[model.DeviceID]map[model.Metric]model.Value

// pollWant is what the last batch of a Poll case must hold.
type pollWant struct {
	devices  []model.Device // nil = the last batch reported no device change
	readings values
	requests int
}

func dp(code string, v any) tuyacloud.DataPoint {
	b, _ := json.Marshal(v)
	return tuyacloud.DataPoint{Code: code, Value: b}
}

func newSource(t *testing.T) (*Source, *MockClient, *MockMetrics) {
	t.Helper()
	ctrl := gomock.NewController(t)
	client := NewMockClient(ctrl)
	m := NewMockMetrics(ctrl)
	s, err := New(Config{Name: "tuya", ListEvery: 2}, client, quiet, m)
	if err != nil {
		t.Fatal(err)
	}
	return s, client, m
}

func TestNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	tests := []struct {
		name    string
		cfg     Config
		client  Client
		wantErr string
	}{
		{name: "valid", cfg: Config{Name: "tuya"}, client: NewMockClient(ctrl)},
		{name: "list cadence defaulted", cfg: Config{Name: "tuya", ListEvery: -1}, client: NewMockClient(ctrl)},
		{name: "name required", cfg: Config{}, client: NewMockClient(ctrl), wantErr: "source name is required"},
		{name: "client required", cfg: Config{Name: "tuya"}, wantErr: "client is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.cfg, tt.client, quiet, NewMockMetrics(ctrl))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if s.cfg.ListEvery <= 0 {
					t.Errorf("ListEvery = %d, want a positive default", s.cfg.ListEvery)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSourceName(t *testing.T) {
	s, _, _ := newSource(t)
	if got := s.Name(); got != "tuya" {
		t.Errorf("Name() = %q", got)
	}
}

func TestSourcePoll(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		setup   func(c *MockClient)
		polls   int
		between time.Duration // virtual time between polls
		age     time.Duration // how far before the last poll the readings are stamped (the device's update time)
		want    pollWant
		wantErr string
	}{
		{
			name: "devices, online flags and scaled points",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office, kitchen, strip}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22", "bf06", "bf21").Return(map[string]tuyacloud.Status{
					"bf22": {dp("va_temperature", 224), dp("va_humidity", 48), dp("battery_percentage", 87), dp("temp_alarm", "cancel"), dp("temp_unit_convert", "c")},
					"bf21": {dp("switch_1", true), dp("countdown_1", 0)},
				}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil)
				c.EXPECT().Specification(gomock.Any(), "bf21").Return(tuyacloud.Specification{}, nil)
			},
			polls: 1,
			want: pollWant{
				devices: []model.Device{
					{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor},
					{ID: "tuya:bf06", Name: "Кухня", Kind: model.KindClimateSensor},
					{ID: "tuya:bf21", Name: "Розетка", Kind: model.KindOutlet},
				},
				readings: values{
					"tuya:bf22": {
						model.Online: model.BoolValue(true), model.Temperature: model.NumberValue(22.4),
						model.Humidity: model.NumberValue(48), model.Battery: model.NumberValue(87),
						model.TemperatureAlarm: model.TextValue("cancel"),
					},
					"tuya:bf06": {model.Online: model.BoolValue(false)},
					"tuya:bf21": {model.Online: model.BoolValue(true), model.Power: model.BoolValue(true)},
				},
				requests: 4, // listing, one status batch, two specifications
			},
		},
		{
			name: "specification cached, listing not repeated between due polls",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil).Times(1)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", 210)}}, nil).Times(2)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil).Times(1)
			},
			polls: 2,
			want: pollWant{
				devices:  nil, // the second poll: unchanged
				readings: values{"tuya:bf22": {model.Temperature: model.NumberValue(21)}},
				requests: 4, // 1 listing, 2 status batches, 1 specification
			},
		},
		{
			name: "listing repeats when due",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil).Times(2) // polls 1 and 3
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", 210)}}, nil).Times(3)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil).Times(1)
			},
			polls: 3,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor}},
				readings: values{"tuya:bf22": {model.Online: model.BoolValue(true), model.Temperature: model.NumberValue(21)}},
				requests: 6,
			},
		},
		{
			name: "specification failure falls back to default scales, retries after the hold-off",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil).Times(1)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", 210), dp("temp_alarm", "upperalarm")}}, nil).Times(2)
				gomock.InOrder(
					c.EXPECT().Specification(gomock.Any(), "bf22").Return(tuyacloud.Specification{}, boom),
					c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil),
				)
			},
			polls:   2,
			between: specRetryAfter,
			want: pollWant{
				devices:  nil,
				readings: values{"tuya:bf22": {model.Temperature: model.NumberValue(21), model.TemperatureAlarm: model.TextValue("upperalarm")}},
				requests: 5,
			},
		},
		{
			name: "specification failure is not retried before the hold-off",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil).Times(1)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", 210), dp("va_humidity", 48)}}, nil).Times(2)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(tuyacloud.Specification{}, boom).Times(1)
			},
			polls:   2,
			between: time.Minute,
			want: pollWant{
				devices:  nil,
				readings: values{"tuya:bf22": {model.Temperature: model.NumberValue(21), model.Humidity: model.NumberValue(48)}},
				requests: 4, // listing, two status batches, one specification attempt
			},
		},
		{
			name: "thermostat: setpoint, valve as heating, default scales without a specification",
			setup: func(c *MockClient) {
				thermo := tuyacloud.Device{ID: "bfth", Name: "Thermostat", Category: "wk", ProductID: "wkp", Online: true}
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{thermo}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bfth").Return(map[string]tuyacloud.Status{
					"bfth": {dp("temp_current", 190), dp("temp_set", 215), dp("switch", true), dp("valve_state", "open"), dp("mode", "manual")},
				}, nil)
				c.EXPECT().Specification(gomock.Any(), "bfth").Return(tuyacloud.Specification{}, boom) // no spec: default scales
			},
			polls: 1,
			want: pollWant{
				devices: []model.Device{{ID: "tuya:bfth", Name: "Thermostat", Kind: model.KindThermostat}},
				readings: values{"tuya:bfth": {
					model.Online: model.BoolValue(true), model.Temperature: model.NumberValue(19), model.TargetTemperature: model.NumberValue(21.5),
					model.Power: model.BoolValue(true), model.Heating: model.BoolValue(true),
				}},
				requests: 3,
			},
		},
		{
			name: "metering outlet: power draw scaled by the specification",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{strip}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf21").Return(map[string]tuyacloud.Status{"bf21": {dp("switch_1", true), dp("cur_power", 285), dp("add_ele", 3)}}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf21").Return(tuyacloud.Specification{Status: []tuyacloud.Definition{
					{Code: "cur_power", Type: "Integer", Values: `{"unit":"W","min":0,"max":50000,"scale":1,"step":1}`},
				}}, nil)
			},
			polls: 1,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf21", Name: "Розетка", Kind: model.KindOutlet}},
				readings: values{"tuya:bf21": {model.Online: model.BoolValue(true), model.Power: model.BoolValue(true), model.PowerDraw: model.NumberValue(28.5)}},
				requests: 3,
			},
		},
		{
			name: "readings stamped with the device's update time",
			setup: func(c *MockClient) {
				seen := office
				seen.UpdateTime = time.Now().Add(-time.Hour).Unix()
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{seen}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", 210)}}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil)
			},
			polls: 1,
			age:   time.Hour,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor}},
				readings: values{"tuya:bf22": {model.Online: model.BoolValue(true), model.Temperature: model.NumberValue(21)}},
				requests: 3,
			},
		},
		{
			name: "an update time from the future is clamped to the poll",
			setup: func(c *MockClient) {
				seen := office
				seen.UpdateTime = time.Now().Add(time.Hour).Unix()
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{seen}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {}}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil)
			},
			polls: 1,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor}},
				readings: values{"tuya:bf22": {model.Online: model.BoolValue(true)}},
				requests: 3,
			},
		},
		{
			name: "unknown code and wrong types dropped",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{"bf22": {dp("va_temperature", "warm"), dp("temp_alarm", 5), dp("switch", "on"), dp("mystery", 1)}}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf22").Return(climateSpec, nil)
			},
			polls: 1,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor}},
				readings: values{"tuya:bf22": {model.Online: model.BoolValue(true)}},
				requests: 3,
			},
		},
		{
			name: "code missing from the specification is unscaled",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{strip}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf21").Return(map[string]tuyacloud.Status{"bf21": {dp("temp_current", 25)}}, nil)
				c.EXPECT().Specification(gomock.Any(), "bf21").Return(tuyacloud.Specification{Status: []tuyacloud.Definition{{Code: "temp_current", Type: "Integer", Values: `not json`}}}, nil)
			},
			polls: 1,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf21", Name: "Розетка", Kind: model.KindOutlet}},
				readings: values{"tuya:bf21": {model.Online: model.BoolValue(true), model.Temperature: model.NumberValue(25)}},
				requests: 3,
			},
		},
		{
			name: "no devices",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return(nil, nil)
			},
			polls: 1,
			want:  pollWant{devices: []model.Device{}, readings: values{}, requests: 1},
		},
		{
			name: "listing fails",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return(nil, boom)
			},
			polls:   1,
			wantErr: "tuya: list devices: boom",
		},
		{
			name: "listing retried right after a failure",
			setup: func(c *MockClient) {
				gomock.InOrder(
					c.EXPECT().Devices(gomock.Any()).Return(nil, boom),
					c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil),
				)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(map[string]tuyacloud.Status{}, nil)
			},
			polls: 2,
			want: pollWant{
				devices:  []model.Device{{ID: "tuya:bf22", Name: "кабинет", Kind: model.KindClimateSensor}},
				readings: values{"tuya:bf22": {model.Online: model.BoolValue(true)}},
				requests: 3,
			},
		},
		{
			name: "status fails",
			setup: func(c *MockClient) {
				c.EXPECT().Devices(gomock.Any()).Return([]tuyacloud.Device{office}, nil)
				c.EXPECT().DevicesStatus(gomock.Any(), "bf22").Return(nil, boom)
			},
			polls:   1,
			wantErr: "tuya: read status: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A bubble: the wall clock stands still, so reading stamps are exact.
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				s, client, m := newSource(t)
				requests := 0
				m.EXPECT().IncRequest("tuya").Do(func(string) { requests++ }).AnyTimes()
				tt.setup(client)
				var (
					batch source.Batch
					err   error
				)
				for i := range tt.polls {
					if i > 0 {
						time.Sleep(tt.between)
					}
					batch, err = s.Poll(t.Context())
				}
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
					}
					if !errors.Is(err, boom) {
						t.Error("error does not wrap the client's error")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				assertDevices(t, batch.Devices, tt.want.devices)
				assertReadings(t, batch.Readings, tt.want.readings, start.Add(tt.between*time.Duration(tt.polls-1)).Add(-tt.age)) // stamped by the last poll, minus the device's age
				if requests != tt.want.requests {
					t.Errorf("requests counted = %d, want %d", requests, tt.want.requests)
				}
			})
		})
	}
}

func assertDevices(t *testing.T, got, want []model.Device) {
	t.Helper()
	if len(got) != len(want) || (got == nil) != (want == nil) {
		t.Fatalf("devices = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("device %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func assertReadings(t *testing.T, readings []model.Reading, want values, stamp time.Time) {
	t.Helper()
	got := values{}
	for _, r := range readings {
		if got[r.Device] == nil {
			got[r.Device] = map[model.Metric]model.Value{}
		}
		got[r.Device][r.Metric] = r.Value
		if !r.At.Equal(stamp) {
			t.Errorf("reading %s/%s stamped %v, want %v", r.Device, r.Metric, r.At, stamp)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("readings for %d devices, want %d: %v", len(got), len(want), got)
	}
	for id, wantMetrics := range want {
		if len(got[id]) != len(wantMetrics) {
			t.Errorf("%s: metrics = %v, want %v", id, got[id], wantMetrics)
			continue
		}
		for metric, wantValue := range wantMetrics {
			if v, ok := got[id][metric]; !ok || !v.Equal(wantValue) {
				t.Errorf("%s/%s = %v, want %v", id, metric, v, wantValue)
			}
		}
	}
}
