package core

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/state"
)

// persist writes one batch under the write policy: the changed readings as
// the state holds them (re-stamped when the vendor's stamp did not move),
// plus a heartbeat row at `now` for series that have gone Heartbeat without
// one, plus the devices and events as given — one transaction. Nothing is
// written without a store or while the clock is not trusted.
func (c *Core) persist(devices []model.Device, readings []model.Reading, changes []state.Change, events []model.Event, now time.Time) {
	c.mu.Lock()
	store := c.store
	ctx := c.ctx
	if store == nil {
		c.mu.Unlock()
		return
	}
	if !c.clockOK(now) {
		c.mu.Unlock()
		return
	}
	changed := make(map[seriesKey]bool, len(changes))
	write := make([]model.Reading, 0, len(changes))
	written := make([]seriesKey, 0, len(changes)) // stamped only once the write succeeded
	for _, ch := range changes {
		key := seriesKey{ch.Device, ch.Metric}
		changed[key] = true
		write = append(write, ch.New)
		written = append(written, key)
	}
	for _, r := range readings {
		key := seriesKey{r.Device, r.Metric}
		if changed[key] {
			continue
		}
		if last, seen := c.lastWritten[key]; !seen || now.Sub(last) >= c.cfg.Heartbeat {
			r.At = now // a heartbeat carries the value forward to this moment
			write = append(write, r)
			written = append(written, key)
		}
	}
	c.mu.Unlock()
	if len(devices) == 0 && len(write) == 0 && len(events) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, persistTimeout)
	defer cancel()
	if err := store.Persist(ctx, devices, write, events, now); err != nil {
		// The rows are lost, but not the heartbeat: with nothing stamped,
		// the next batch writes the value again.
		c.log.Error("persist failed", "devices", len(devices), "readings", len(write), "events", len(events), "err", err)
		c.metrics.IncStoreError("persist")
		return
	}
	c.mu.Lock()
	for _, key := range written {
		c.lastWritten[key] = now
	}
	c.mu.Unlock()
	c.log.Debug("persisted", "devices", len(devices), "readings", len(write), "events", len(events))
}

// restore seeds the state from the store: the last known picture, so the
// first frame after a restart is not blank.
func (c *Core) restore(ctx context.Context) {
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, persistTimeout)
	defer cancel()
	devices, readings, err := store.Restore(ctx)
	if err != nil {
		c.log.Error("restore failed, starting empty", "err", err)
		c.metrics.IncStoreError("restore")
		return
	}
	devices, readings, dropped := c.configured(devices, readings)
	if len(dropped) > 0 {
		c.log.Info("restore skipped devices of sources no longer configured", "devices", dropped)
	}
	if len(devices) == 0 && len(readings) == 0 {
		return
	}
	c.state.Restore(devices, readings)
	c.log.Info("state restored", "devices", len(devices), "readings", len(readings))
}

// configured keeps what belongs to a source with a poller and returns the
// IDs it dropped: a section renamed or removed since the rows were written
// has nobody to list its devices again, so they would sit on the display
// for good. Their history stays in the file.
func (c *Core) configured(devices []model.Device, readings []model.Reading) ([]model.Device, []model.Reading, []model.DeviceID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	known := func(id model.DeviceID) bool {
		_, ok := c.pollers[id.Source()]
		return ok
	}
	dropped := map[model.DeviceID]bool{}
	devices = slices.DeleteFunc(devices, func(d model.Device) bool {
		if known(d.ID) {
			return false
		}
		dropped[d.ID] = true
		return true
	})
	readings = slices.DeleteFunc(readings, func(r model.Reading) bool {
		if known(r.Device) {
			return false
		}
		dropped[r.Device] = true
		return true
	})
	return devices, readings, slices.Sorted(maps.Keys(dropped))
}

// housekeeping prunes rows older than the retention once a day, starting
// now, while the run lasts.
func (c *Core) housekeeping(ctx context.Context) {
	for {
		c.prune(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(pruneEvery):
		}
	}
}

// prune deletes what is older than the retention.
func (c *Core) prune(ctx context.Context) {
	c.mu.Lock()
	store := c.store
	ok := store != nil && c.clockOK(time.Now())
	c.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, persistTimeout)
	defer cancel()
	before := time.Now().Add(-c.cfg.Retention)
	n, err := store.Prune(ctx, before)
	if err != nil {
		c.log.Error("prune failed", "err", err)
		c.metrics.IncStoreError("prune")
		return
	}
	c.log.Info("pruned", "rows", n, "before", before)
}

// clockOK reports whether the clock can be trusted for history: not
// earlier than the floor. Warned once; the caller holds the lock.
func (c *Core) clockOK(now time.Time) bool {
	if now.Before(c.cfg.ClockFloor) {
		if !c.clockWarned {
			c.log.Warn("clock is behind the build, not persisting until it is synced", "now", now, "floor", c.cfg.ClockFloor)
			c.clockWarned = true
		}
		return false
	}
	return true
}
