package source

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/mshaulsky/domovoi/internal/model"
)

// quiet is a logger that keeps test output clean.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// quietMetrics accepts any metrics call; tests that care set expectations
// on their own mock instead.
func quietMetrics(ctrl *gomock.Controller) *MockMetrics {
	m := NewMockMetrics(ctrl)
	m.EXPECT().IncPollSuccess(gomock.Any()).AnyTimes()
	m.EXPECT().IncPollError(gomock.Any()).AnyTimes()
	m.EXPECT().ObservePollDuration(gomock.Any(), gomock.Any()).AnyTimes()
	return m
}

func namedSource(ctrl *gomock.Controller) *MockSource {
	src := NewMockSource(ctrl)
	src.EXPECT().Name().Return("tuya").AnyTimes()
	return src
}

func TestNewPoller(t *testing.T) {
	ctrl := gomock.NewController(t)
	tests := []struct {
		name string
		cfg  PollerConfig
		want PollerConfig
	}{
		{
			name: "defaults filled",
			cfg:  PollerConfig{Interval: time.Minute},
			want: PollerConfig{Interval: time.Minute, Timeout: time.Minute, MaxBackoff: DefaultMaxBackoff},
		},
		{
			name: "explicit kept",
			cfg:  PollerConfig{Interval: time.Minute, Timeout: 10 * time.Second, MaxBackoff: time.Hour},
			want: PollerConfig{Interval: time.Minute, Timeout: 10 * time.Second, MaxBackoff: time.Hour},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPoller(namedSource(ctrl), NewMockSink(ctrl), tt.cfg, quiet, quietMetrics(ctrl))
			if p.cfg != tt.want {
				t.Errorf("cfg = %+v, want %+v", p.cfg, tt.want)
			}
		})
	}
}

func TestPollerOnce(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		polls   []error // outcome of each successive Poll
		expect  func(sink *MockSink, m *MockMetrics, batch Batch)
		wantErr string
	}{
		{
			name:  "success delivers the batch",
			polls: []error{nil},
			expect: func(sink *MockSink, m *MockMetrics, batch Batch) {
				m.EXPECT().ObservePollDuration("tuya", 3*time.Second)
				m.EXPECT().IncPollSuccess("tuya")
				sink.EXPECT().Batch("tuya", batch)
			},
		},
		{
			name:  "failure reports health",
			polls: []error{boom},
			expect: func(sink *MockSink, m *MockMetrics, _ Batch) {
				m.EXPECT().ObservePollDuration("tuya", 3*time.Second)
				m.EXPECT().IncPollError("tuya")
				sink.EXPECT().Health("tuya", boom)
			},
			wantErr: "poll tuya: boom",
		},
		{
			name:  "recovery logs once; the batch carries the healthy mark",
			polls: []error{boom, boom, nil, nil},
			expect: func(sink *MockSink, m *MockMetrics, batch Batch) {
				m.EXPECT().ObservePollDuration("tuya", 3*time.Second).Times(4)
				m.EXPECT().IncPollError("tuya").Times(2)
				m.EXPECT().IncPollSuccess("tuya").Times(2)
				gomock.InOrder(
					sink.EXPECT().Health("tuya", boom),
					sink.EXPECT().Health("tuya", boom),
					sink.EXPECT().Batch("tuya", batch),
					sink.EXPECT().Batch("tuya", batch),
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				src := namedSource(ctrl)
				sink := NewMockSink(ctrl)
				m := NewMockMetrics(ctrl)
				batch := Batch{Readings: []model.Reading{{Device: "tuya:a", Metric: model.Temperature, Value: model.NumberValue(21), At: time.Now()}}}
				for _, outcome := range tt.polls {
					src.EXPECT().Poll(gomock.Any()).DoAndReturn(func(ctx context.Context) (Batch, error) {
						if _, ok := ctx.Deadline(); !ok {
							t.Error("poll context has no deadline")
						}
						time.Sleep(3 * time.Second) // virtual: the poll "takes" three seconds
						if outcome != nil {
							return Batch{}, outcome
						}
						return batch, nil
					})
				}
				tt.expect(sink, m, batch)
				p := NewPoller(src, sink, PollerConfig{Interval: time.Minute}, quiet, m)
				var last error
				for range tt.polls {
					last = p.Once(t.Context())
				}
				if tt.wantErr == "" {
					if last != nil {
						t.Fatalf("Once = %v", last)
					}
					return
				}
				if last == nil || !strings.Contains(last.Error(), tt.wantErr) {
					t.Fatalf("Once = %v, want containing %q", last, tt.wantErr)
				}
				if !errors.Is(last, boom) {
					t.Error("error does not wrap the source's error")
				}
			})
		})
	}
}

func TestPollerRun(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name       string
		outcomes   []error
		maxBackoff time.Duration
		wantWaits  []time.Duration // the wait scheduled after each poll
	}{
		{
			name:      "steady interval while healthy",
			outcomes:  []error{nil, nil, nil},
			wantWaits: []time.Duration{time.Minute, time.Minute, time.Minute},
		},
		{
			name:      "backoff doubles and resets",
			outcomes:  []error{boom, boom, boom, nil, nil},
			wantWaits: []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, time.Minute, time.Minute},
		},
		{
			name:       "backoff capped",
			outcomes:   []error{boom, boom, boom, boom},
			maxBackoff: 3 * time.Minute,
			wantWaits:  []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute, 3 * time.Minute},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				src := namedSource(ctrl)
				sink := NewMockSink(ctrl)
				sink.EXPECT().Batch(gomock.Any(), gomock.Any()).AnyTimes()
				sink.EXPECT().Health(gomock.Any(), gomock.Any()).AnyTimes()
				var polls atomic.Int32
				for _, outcome := range tt.outcomes {
					src.EXPECT().Poll(gomock.Any()).DoAndReturn(func(context.Context) (Batch, error) {
						polls.Add(1)
						return Batch{}, outcome
					})
				}
				p := NewPoller(src, sink, PollerConfig{Interval: time.Minute, MaxBackoff: tt.maxBackoff}, quiet, quietMetrics(ctrl))
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan error, 1)
				go func() { done <- p.Run(ctx) }()

				for i, want := range tt.wantWaits {
					synctest.Wait() // the poll ran and the poller parked on its timer
					if got := int(polls.Load()); got != i+1 {
						t.Fatalf("after poll %d: %d polls happened", i+1, got)
					}
					// One second short of the wait must not wake it; the last
					// second must.
					time.Sleep(want - time.Second)
					synctest.Wait()
					if got := int(polls.Load()); got != i+1 {
						t.Fatalf("poll %d fired before %v elapsed", i+2, want)
					}
					if i < len(tt.wantWaits)-1 {
						time.Sleep(time.Second)
					}
				}
				cancel()
				if err := <-done; err != nil {
					t.Errorf("Run = %v, want nil on cancellation", err)
				}
			})
		})
	}
}

// TestPollerOnceCancelled: a poll cut short by shutdown is neither a failure
// nor a recovery — the sink and the metrics hear nothing about it.
func TestPollerOnceCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		src := namedSource(ctrl)
		sink := NewMockSink(ctrl)
		m := NewMockMetrics(ctrl)
		m.EXPECT().ObservePollDuration("tuya", gomock.Any())
		ctx, cancel := context.WithCancel(t.Context())
		src.EXPECT().Poll(gomock.Any()).DoAndReturn(func(ctx context.Context) (Batch, error) {
			cancel()
			return Batch{}, ctx.Err()
		})
		p := NewPoller(src, sink, PollerConfig{Interval: time.Minute}, quiet, m)
		if err := p.Once(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Once = %v, want context.Canceled", err)
		}
		if p.failures != 0 {
			t.Error("a cancelled poll counted as a failure")
		}
	})
}
