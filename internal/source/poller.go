package source

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Poller adapts a Source to the Sink: one goroutine, Poll every Interval,
// Batch on success, Health on failure with exponential backoff.
type Poller struct {
	src      Source
	sink     Sink
	cfg      PollerConfig
	log      *slog.Logger
	metrics  Metrics
	failures int
}

// PollerConfig tunes the schedule.
type PollerConfig struct {
	Interval   time.Duration // between successful polls
	Timeout    time.Duration // per poll; defaults to Interval
	MaxBackoff time.Duration // cap on the wait after repeated failures; defaults to DefaultMaxBackoff
}

// DefaultMaxBackoff caps the wait between retries of a failing source.
const DefaultMaxBackoff = 15 * time.Minute

// NewPoller wires a source to a sink.
func NewPoller(src Source, sink Sink, cfg PollerConfig, log *slog.Logger, m Metrics) *Poller {
	if cfg.Timeout <= 0 {
		cfg.Timeout = cfg.Interval
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = DefaultMaxBackoff
	}
	return &Poller{
		src:     src,
		sink:    sink,
		cfg:     cfg,
		log:     log.With("component", "poller", "source", src.Name()),
		metrics: m,
	}
}

// Run polls immediately, then keeps polling until ctx is done. It returns
// nil on cancellation: stopping is not a failure.
func (p *Poller) Run(ctx context.Context) error {
	for {
		_ = p.Once(ctx) // outcome already reported to the sink and the log
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.delay()):
		}
	}
}

// Once performs a single poll and reports it to the sink. The error is
// returned for callers that want it (the -once flag); Run only logs it.
func (p *Poller) Once(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	start := time.Now()
	batch, err := p.src.Poll(pollCtx)
	p.metrics.ObservePollDuration(p.src.Name(), time.Since(start))
	if err != nil {
		if ctx.Err() != nil { // stopping, not failing: nothing to report
			return fmt.Errorf("poll %s: %w", p.src.Name(), ctx.Err())
		}
		p.failures++
		p.metrics.IncPollError(p.src.Name())
		p.sink.Health(p.src.Name(), err)
		if p.failures == 1 {
			p.log.Warn("poll failed", "err", err)
		} else {
			p.log.Debug("poll failed again", "err", err, "failures", p.failures, "next_in", p.delay())
		}
		return fmt.Errorf("poll %s: %w", p.src.Name(), err)
	}
	p.metrics.IncPollSuccess(p.src.Name())
	p.sink.Batch(p.src.Name(), batch) // a batch also marks the source healthy
	if p.failures > 0 {
		p.log.Info("poll recovered", "after_failures", p.failures)
		p.failures = 0
	}
	p.log.Debug("polled", "devices", len(batch.Devices), "readings", len(batch.Readings))
	return nil
}

// delay is the wait before the next poll: the interval when healthy,
// doubling per consecutive failure up to MaxBackoff.
func (p *Poller) delay() time.Duration {
	d := p.cfg.Interval
	for i := 1; i < p.failures && d < p.cfg.MaxBackoff; i++ {
		d *= 2
	}
	return min(d, p.cfg.MaxBackoff)
}
