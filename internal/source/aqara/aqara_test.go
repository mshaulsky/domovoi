package aqara

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/aqaramcp"

	"github.com/mshaulsky/domovoi/internal/model"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// The user's inventory, as the status table reports it.
var (
	hub = aqaramcp.Device{
		EndpointID:   "Aqr~hub",
		DeviceName:   "Центр умного дома M3",
		Type:         aqaramcp.TypeHub,
		PositionName: "Комната по умолчанию",
	}
	leak1 = aqaramcp.Device{
		EndpointID:   "Aqr~leak1",
		DeviceName:   "Протечка стиралки",
		Type:         aqaramcp.TypeWaterLeakSensor,
		PositionName: "Прихожая",
	}
	leak2 = aqaramcp.Device{
		EndpointID:   "Aqr~leak2",
		DeviceName:   "Протечка ванная",
		Type:         aqaramcp.TypeWaterLeakSensor,
		PositionName: "Ванная",
	}
	washer = aqaramcp.Device{
		EndpointID:   "Aqr~washer",
		DeviceName:   "Розетка стиралка",
		Type:         aqaramcp.TypeOutlet,
		PositionName: "Прихожая",
	}
	button = aqaramcp.Device{
		EndpointID:   "Aqr~button",
		DeviceName:   "Кнопка замка",
		Type:         aqaramcp.TypeButton,
		PositionName: "Прихожая",
	}
	lock = aqaramcp.Device{
		EndpointID:   "Aqr~lock",
		DeviceName:   "Дверной замок (U200)",
		Type:         aqaramcp.TypeDoorLock,
		PositionName: "Прихожая",
	}
	curtain = aqaramcp.Device{
		EndpointID:   "Aqr~sw",
		DeviceName:   "Выключатель",
		Type:         "Curtain",
		PositionName: "Зал",
	}
)

// values is the readings of one batch, by device and metric.
type values map[model.DeviceID]map[model.Metric]model.Value

// pollWant is what a Poll must return.
type pollWant struct {
	devices  []model.Device
	readings values
	requests int
}

// byPrint is the device ID the source gives a device: its fingerprint.
func byPrint(d aqaramcp.Device) model.DeviceID {
	return model.NewDeviceID("aqara", d.Fingerprint())
}

// byEndpoint is the fallback ID of a device whose fingerprint is shared.
func byEndpoint(d aqaramcp.Device) model.DeviceID {
	return model.NewDeviceID("aqara", d.EndpointID)
}

func status(d aqaramcp.Device, kv ...string) aqaramcp.DeviceStatus {
	st := aqaramcp.Status{}
	for i := 0; i+1 < len(kv); i += 2 {
		st[kv[i]] = kv[i+1]
	}
	return aqaramcp.DeviceStatus{Device: d, Status: st}
}

func newSource(t *testing.T) (*Source, *MockClient, *MockMetrics) {
	t.Helper()
	ctrl := gomock.NewController(t)
	client := NewMockClient(ctrl)
	m := NewMockMetrics(ctrl)
	s, err := New(Config{Name: "aqara"}, client, quiet, m)
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
		{name: "valid", cfg: Config{Name: "aqara"}, client: NewMockClient(ctrl)},
		{name: "name required", cfg: Config{}, client: NewMockClient(ctrl), wantErr: "source name is required"},
		{name: "client required", cfg: Config{Name: "aqara"}, wantErr: "client is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.cfg, tt.client, quiet, NewMockMetrics(ctrl))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if s.Name() != tt.cfg.Name {
					t.Errorf("Name() = %q, want %q", s.Name(), tt.cfg.Name)
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
	if got := s.Name(); got != "aqara" {
		t.Errorf("Name() = %q", got)
	}
}

func TestSourcePoll(t *testing.T) {
	boom := errors.New("boom")
	on, off := model.BoolValue(true), model.BoolValue(false)
	// The lock and a leak sensor as a later sign-in lists them: new endpoint and position IDs.
	lockAgain, leak1Again := lock, leak1
	lockAgain.EndpointID, lockAgain.PositionID = "Aqr~lock-next", "Aqr~hall-next"
	leak1Again.EndpointID, leak1Again.PositionID = "Aqr~leak1-next", "Aqr~hall-next"
	// leak2Twin has the type, name and room of leak2, so their fingerprints collide.
	leak2Twin := leak2
	leak2Twin.EndpointID = "Aqr~leak3"
	tests := []struct {
		name    string
		setup   func(c *MockClient)
		polls   int
		want    pollWant
		wantErr string
	}{
		{
			name: "the whole home in one call, devices every time",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), aqaramcp.StatusFilter{}).Return([]aqaramcp.DeviceStatus{
					status(hub, "online_offline", "online"),
					status(leak1, "water_leak", "False", "online_offline", "online"),
					status(washer, "on_off", "on", "online_offline", "online"),
					status(button, "online_offline", "offline"),
					status(lock, "lock_state", "1", "online_offline", "online"),
					status(leak2, "water_leak", "True", "online_offline", "online"),
				}, nil).Times(2)
			},
			polls: 2,
			want: pollWant{
				devices: []model.Device{
					{
						ID:   byPrint(hub),
						Name: "Центр умного дома M3",
						Room: "Комната по умолчанию",
						Kind: model.KindHub,
					},
					{
						ID:   byPrint(leak1),
						Name: "Протечка стиралки",
						Room: "Прихожая",
						Kind: model.KindLeakSensor,
					},
					{
						ID:   byPrint(washer),
						Name: "Розетка стиралка",
						Room: "Прихожая",
						Kind: model.KindOutlet,
					},
					{
						ID:   byPrint(button),
						Name: "Кнопка замка",
						Room: "Прихожая",
						Kind: model.KindButton,
					},
					{
						ID:   byPrint(lock),
						Name: "Дверной замок (U200)",
						Room: "Прихожая",
						Kind: model.KindLock,
					},
					{
						ID:   byPrint(leak2),
						Name: "Протечка ванная",
						Room: "Ванная",
						Kind: model.KindLeakSensor,
					},
				},
				readings: values{
					byPrint(hub):    {model.Online: on},
					byPrint(leak1):  {model.Leak: off, model.Online: on},
					byPrint(washer): {model.Power: on, model.Online: on},
					byPrint(button): {model.Online: off},
					byPrint(lock):   {model.Locked: on, model.Online: on},
					byPrint(leak2):  {model.Leak: on, model.Online: on},
				},
				requests: 2,
			},
		},
		{
			name: "IDs come from the fingerprint, so a new sign-in keeps them",
			setup: func(c *MockClient) {
				gomock.InOrder(
					c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{
						status(lock, "lock_state", "1", "online_offline", "online"),
						status(leak1, "water_leak", "False", "online_offline", "online"),
					}, nil),
					c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{
						status(lockAgain, "lock_state", "1", "online_offline", "online"),
						status(leak1Again, "water_leak", "False", "online_offline", "online"),
					}, nil),
				)
			},
			polls: 2,
			want: pollWant{
				devices: []model.Device{
					{
						ID:   byPrint(lock),
						Name: "Дверной замок (U200)",
						Room: "Прихожая",
						Kind: model.KindLock,
					},
					{
						ID:   byPrint(leak1),
						Name: "Протечка стиралки",
						Room: "Прихожая",
						Kind: model.KindLeakSensor,
					},
				},
				readings: values{
					byPrint(lock):  {model.Locked: on, model.Online: on},
					byPrint(leak1): {model.Leak: off, model.Online: on},
				},
				requests: 2,
			},
		},
		{
			name: "devices sharing type, name and room fall back to endpoint IDs",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{
					status(leak2, "water_leak", "False", "online_offline", "online"),
					status(lock, "lock_state", "1", "online_offline", "online"),
					status(leak2Twin, "water_leak", "True", "online_offline", "offline"),
				}, nil).Times(2)
			},
			polls: 2,
			want: pollWant{
				devices: []model.Device{
					{
						ID:   byEndpoint(leak2),
						Name: "Протечка ванная",
						Room: "Ванная",
						Kind: model.KindLeakSensor,
					},
					{
						ID:   byPrint(lock),
						Name: "Дверной замок (U200)",
						Room: "Прихожая",
						Kind: model.KindLock,
					},
					{
						ID:   byEndpoint(leak2Twin),
						Name: "Протечка ванная",
						Room: "Ванная",
						Kind: model.KindLeakSensor,
					},
				},
				readings: values{
					byEndpoint(leak2):     {model.Leak: off, model.Online: on},
					byPrint(lock):         {model.Locked: on, model.Online: on},
					byEndpoint(leak2Twin): {model.Leak: on, model.Online: off},
				},
				requests: 2,
			},
		},
		{
			name: "any lock_state but the locked one reads as unlocked",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{
					status(lock, "lock_state", "0", "online_offline", "online"),
				}, nil)
			},
			polls: 1,
			want: pollWant{
				devices: []model.Device{
					{
						ID:   byPrint(lock),
						Name: "Дверной замок (U200)",
						Room: "Прихожая",
						Kind: model.KindLock,
					},
				},
				readings: values{byPrint(lock): {model.Locked: off, model.Online: on}},
				requests: 1,
			},
		},
		{
			name: "outlet off, unknown keys and bad flags dropped, unknown type is other",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{
					status(
						washer,
						"on_off",
						"off",
						"online_offline",
						"online",
						"power_consumed",
						"12.5",
						"mystery",
						"x",
					),
					status(leak2, "water_leak", "maybe", "online_offline", "online"),
					status(curtain, "online_offline", "online", "curtain_state", "open"),
				}, nil)
			},
			polls: 1,
			want: pollWant{
				devices: []model.Device{
					{ID: byPrint(washer), Name: "Розетка стиралка", Room: "Прихожая", Kind: model.KindOutlet},
					{ID: byPrint(leak2), Name: "Протечка ванная", Room: "Ванная", Kind: model.KindLeakSensor},
					{ID: byPrint(curtain), Name: "Выключатель", Room: "Зал", Kind: model.KindOther},
				},
				readings: values{
					byPrint(washer):  {model.Power: off, model.Online: on},
					byPrint(leak2):   {model.Online: on},
					byPrint(curtain): {model.Online: on},
				},
				requests: 1,
			},
		},
		{
			name: "a device without status still lists",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return([]aqaramcp.DeviceStatus{{Device: button}}, nil)
			},
			polls: 1,
			want: pollWant{
				devices: []model.Device{
					{
						ID:   byPrint(button),
						Name: "Кнопка замка",
						Room: "Прихожая",
						Kind: model.KindButton,
					},
				},
				readings: values{},
				requests: 1,
			},
		},
		{
			name: "empty home",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			polls: 1,
			want:  pollWant{devices: []model.Device{}, readings: values{}, requests: 1},
		},
		{
			name: "statuses fail",
			setup: func(c *MockClient) {
				c.EXPECT().Statuses(gomock.Any(), gomock.Any()).Return(nil, boom)
			},
			polls:   1,
			wantErr: "aqara: read statuses: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A bubble: the wall clock stands still, so reading stamps are exact.
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				s, client, m := newSource(t)
				requests := 0
				m.EXPECT().IncRequest("aqara").Do(func(string) { requests++ }).AnyTimes()
				tt.setup(client)
				// Every poll must answer the same: what a device is called does
				// not depend on how many polls, or sign-ins, came before.
				for range tt.polls {
					batch, err := s.Poll(t.Context())
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
					assertReadings(t, batch.Readings, tt.want.readings, start)
				}
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
