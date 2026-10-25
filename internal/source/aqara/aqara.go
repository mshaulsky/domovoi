package aqara

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
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
	seen       map[string]bool      // status keys dropped, logged once
	lockStates map[string]bool      // lock_state values met, logged once
	shared     map[string]bool      // fingerprints shared by several devices, logged once
	home       string               // the home whose history is read; cleared on a failed read
	runs       map[string]time.Time // automation runs already seen, by name and minute
	primed     bool                 // the history has been read once
	down       bool                 // the last history read failed; warned once, until it recovers
}

// Config is what the source needs; the credential is handled by the caller,
// which builds the client.
type Config struct {
	Name        string         // source name, prefix of its device IDs
	Automations []Automation   // automations whose runs are device events; none, no history is read
	Location    *time.Location // the zone the server speaks its minutes in; nil means time.Local
}

// lockedState is the lock_state value observed on a locked U200. The
// platform does not document the other values; any other value reads as
// unlocked and is logged once so it can be verified.
const lockedState = "1"

// historyWindow is how far back each poll reads the automation history.
const historyWindow = 30 * time.Minute

// kinds maps the platform's device types to device kinds.
var kinds = map[string]model.Kind{
	aqaramcp.TypeDoorLock:        model.KindLock,
	aqaramcp.TypeWaterLeakSensor: model.KindLeakSensor,
	aqaramcp.TypeOutlet:          model.KindOutlet,
	aqaramcp.TypeButton:          model.KindButton,
	aqaramcp.TypeHub:             model.KindHub,
}

// Automation turns the runs of an automation set up in the app into events
// of a device: the history is the only event log the server offers.
type Automation struct {
	Match  string          // a part of the automation's name, which the app spells from trigger and action
	Device model.DeviceID  // the device the event belongs to
	Event  model.EventKind // what a run means
}

// New wires the source to a client.
func New(cfg Config, client Client, log *slog.Logger, m Metrics) (*Source, error) {
	if cfg.Name == "" {
		return nil, errors.New("aqara: source name is required")
	}
	if client == nil {
		return nil, errors.New("aqara: client is required")
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	return &Source{
		cfg:        cfg,
		client:     client,
		log:        log.With("component", "source.aqara", "source", cfg.Name),
		metrics:    m,
		seen:       map[string]bool{},
		lockStates: map[string]bool{},
		shared:     map[string]bool{},
		runs:       map[string]time.Time{},
	}, nil
}

// Name returns the configured source name.
func (s *Source) Name() string {
	return s.cfg.Name
}

// Poll reads every device's status in one call. The answer carries names,
// types and rooms, so Batch.Devices is filled on every poll. Device IDs come
// from the fingerprint, since endpoint IDs change with every sign-in;
// devices that share a fingerprint keep their endpoint IDs instead. With
// automations configured, their recent runs become Batch.Events.
func (s *Source) Poll(ctx context.Context) (source.Batch, error) {
	now := time.Now()
	s.metrics.IncRequest(s.cfg.Name)
	statuses, err := s.client.Statuses(ctx, aqaramcp.StatusFilter{})
	if err != nil {
		return source.Batch{}, fmt.Errorf("aqara: read statuses: %w", err)
	}
	// Two passes, so which devices keep a fingerprint does not depend on
	// the listing order.
	endpoints := map[string][]string{}
	for _, st := range statuses {
		fp := st.Fingerprint()
		endpoints[fp] = append(endpoints[fp], st.EndpointID)
	}
	batch := source.Batch{Devices: make([]model.Device, 0, len(statuses))}
	for _, st := range statuses {
		fp := st.Fingerprint()
		id := model.NewDeviceID(s.cfg.Name, fp)
		if shared := endpoints[fp]; len(shared) > 1 {
			// A fingerprint several devices share identifies none of them.
			s.sharedFingerprint(fp, shared)
			id = model.NewDeviceID(s.cfg.Name, st.EndpointID)
		}
		batch.Devices = append(batch.Devices, model.Device{
			ID:   id,
			Name: st.DeviceName,
			Room: st.PositionName,
			Kind: kindOf(st.Type),
		})
		for key, value := range st.Status {
			r, ok := s.convert(id, key, value, now)
			if ok {
				batch.Readings = append(batch.Readings, r)
			}
		}
	}
	if len(s.cfg.Automations) > 0 {
		batch.Events = s.events(ctx, now)
	}
	return batch, nil
}

// events reads the automation runs of the last historyWindow and turns the new ones into events.
// The first read only records what already ran, so a restart does not emit it again.
func (s *Source) events(ctx context.Context, now time.Time) []model.Event {
	s.mu.Lock()
	home := s.home
	s.mu.Unlock()
	if home == "" {
		s.metrics.IncRequest(s.cfg.Name)
		homes, err := s.client.Homes(ctx)
		if err == nil && len(homes) == 0 {
			err = errors.New("the account has no home")
		}
		if err != nil {
			s.mu.Lock()
			s.failed("homes unavailable", err)
			s.mu.Unlock()
			return nil
		}
		home = homes[0].ID
	}
	now = now.In(s.cfg.Location)
	s.metrics.IncRequest(s.cfg.Name)
	filter := aqaramcp.AutomationFilter{PositionIDs: []string{home}}
	runs, err := s.client.AutomationRuns(ctx, now.Add(-historyWindow), now, filter)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		// The home ID may have rotated with the key: look it up again next time.
		s.home = ""
		s.failed("automation history unavailable", err)
		return nil
	}
	if s.down {
		s.log.Info("automation history recovered")
		s.down = false
	}
	s.home = home
	var events []model.Event
	for _, run := range runs {
		a, ok := s.automation(run.Automation)
		if run.Failed || !ok {
			continue
		}
		key := run.Automation + "|" + run.At.Format("2006-01-02 15:04")
		if _, seen := s.runs[key]; seen {
			continue
		}
		s.runs[key] = run.At
		if s.primed {
			events = append(events, model.Event{Device: a.Device, Kind: a.Event, At: run.At})
		}
	}
	s.primed = true
	cutoff := now.Add(-historyWindow - time.Minute)
	maps.DeleteFunc(s.runs, func(_ string, at time.Time) bool { return at.Before(cutoff) })
	return events
}

// failed notes a failed history read, under s.mu: a warning on the first, silence until it recovers.
func (s *Source) failed(msg string, err error) {
	if !s.down {
		s.log.Warn(msg, "err", err)
	}
	s.down = true
}

// automation returns the first configured automation whose match the name contains.
func (s *Source) automation(name string) (Automation, bool) {
	for _, a := range s.cfg.Automations {
		if strings.Contains(name, a.Match) {
			return a, true
		}
	}
	return Automation{}, false
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

// sharedFingerprint warns once per fingerprint that several devices share
// it and so fall back to IDs that change with the next sign-in.
func (s *Source) sharedFingerprint(fp string, endpointIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shared[fp] {
		return
	}
	s.shared[fp] = true
	s.log.Warn("devices share name, type and room; rename one in the app so they keep stable IDs",
		"fingerprint", fp, "endpoints", endpointIDs)
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
