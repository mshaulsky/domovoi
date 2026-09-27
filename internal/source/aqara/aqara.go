package aqara

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mshaulsky/aqaramcp"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
)

// Source polls an Aqara Home account through the MCP server.
type Source struct {
	cfg     Config
	client  Client
	log     *slog.Logger
	metrics Metrics

	mu         sync.Mutex
	seen       map[string]bool // status keys dropped, logged once
	lockStates map[string]bool // lock_state values met, logged once
}

// Config is what the source needs; the credential is handled by the caller,
// which builds the client.
type Config struct {
	Name string // source name, prefix of its device IDs
}

// lockedState is the lock_state value observed on a locked U200. The
// platform does not document the other values; any other value reads as
// unlocked and is logged once so it can be verified.
const lockedState = "1"

// kinds maps the platform's device types to device kinds.
var kinds = map[string]model.Kind{
	aqaramcp.TypeDoorLock:        model.KindLock,
	aqaramcp.TypeWaterLeakSensor: model.KindLeakSensor,
	aqaramcp.TypeOutlet:          model.KindOutlet,
	aqaramcp.TypeButton:          model.KindButton,
	aqaramcp.TypeHub:             model.KindHub,
}

// New wires the source to a client.
func New(cfg Config, client Client, log *slog.Logger, m Metrics) (*Source, error) {
	if cfg.Name == "" {
		return nil, errors.New("aqara: source name is required")
	}
	if client == nil {
		return nil, errors.New("aqara: client is required")
	}
	return &Source{
		cfg:        cfg,
		client:     client,
		log:        log.With("component", "source.aqara", "source", cfg.Name),
		metrics:    m,
		seen:       map[string]bool{},
		lockStates: map[string]bool{},
	}, nil
}

// Name returns the configured source name.
func (s *Source) Name() string {
	return s.cfg.Name
}

// Poll reads every device's status in one call. The answer carries names,
// types and rooms, so Batch.Devices is filled on every poll.
func (s *Source) Poll(ctx context.Context) (source.Batch, error) {
	now := time.Now()
	s.metrics.IncRequest(s.cfg.Name)
	statuses, err := s.client.Statuses(ctx, aqaramcp.StatusFilter{})
	if err != nil {
		return source.Batch{}, fmt.Errorf("aqara: read statuses: %w", err)
	}
	batch := source.Batch{Devices: make([]model.Device, 0, len(statuses))}
	for _, st := range statuses {
		id := model.NewDeviceID(s.cfg.Name, st.EndpointID)
		batch.Devices = append(batch.Devices, model.Device{ID: id, Name: st.DeviceName, Room: st.PositionName, Kind: kindOf(st.Type)})
		for key, value := range st.Status {
			r, ok := s.convert(id, key, value, now)
			if ok {
				batch.Readings = append(batch.Readings, r)
			}
		}
	}
	return batch, nil
}

// convert maps one status key into a reading, or reports that it is dropped.
func (s *Source) convert(id model.DeviceID, key, value string, at time.Time) (model.Reading, bool) {
	r := model.Reading{Device: id, At: at}
	switch key {
	case aqaramcp.KeyOnline:
		r.Metric, r.Value = model.Online, model.BoolValue(value == "online")
	case aqaramcp.KeyLockState:
		s.noteLockState(id, value)
		r.Metric, r.Value = model.Locked, model.BoolValue(value == lockedState)
	case aqaramcp.KeyWaterLeak:
		wet, err := aqaramcp.Status{key: value}.Bool(key)
		if err != nil {
			s.log.Debug("status value is not a flag", "device", id, "key", key, "value", value)
			return model.Reading{}, false
		}
		r.Metric, r.Value = model.Leak, model.BoolValue(wet)
	case aqaramcp.KeyOnOff:
		r.Metric, r.Value = model.Power, model.BoolValue(value == "on")
	default:
		s.dropped(key)
		return model.Reading{}, false
	}
	return r, true
}

// noteLockState logs each distinct lock_state value once, so the unlocked
// value — undocumented by the platform — can be confirmed from the log.
func (s *Source) noteLockState(id model.DeviceID, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockStates[value] {
		return
	}
	s.lockStates[value] = true
	s.log.Info("lock_state value observed", "device", id, "value", value, "read_as_locked", value == lockedState)
}

// dropped logs an unmapped status key once.
func (s *Source) dropped(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	s.log.Debug("status key maps to no metric, dropped", "key", key)
}

func kindOf(deviceType string) model.Kind {
	if k, ok := kinds[deviceType]; ok {
		return k
	}
	return model.KindOther
}
