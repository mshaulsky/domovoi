package application

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeCore is a pinger answering with a fixed error and recording when it
// was asked.
type fakeCore struct {
	err   error
	mu    sync.Mutex
	pings []time.Time
}

// fakeCoordinator is a progressor with a fixed promise.
type fakeCoordinator struct {
	name string
	due  time.Time
}

func (f *fakeCore) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings = append(f.pings, time.Now())
	return f.err
}

func (f fakeCoordinator) Name() string {
	return f.name
}

func (f fakeCoordinator) Due() time.Time {
	return f.due
}

func TestLivenessCheck(t *testing.T) {
	stuck := errors.New("core: ping: context deadline exceeded")
	tests := []struct {
		name    string
		pingErr error
		dues    []time.Duration // per coordinator, after now; 0: the zero time
		wantErr string          // a fragment: the due time is printed in the local zone
	}{
		{
			name: "all good",
			dues: []time.Duration{time.Minute, time.Second},
		},
		{
			name:    "core does not answer",
			pingErr: stuck,
			dues:    []time.Duration{time.Minute},
			wantErr: "core: ping",
		},
		{
			name:    "one display overdue",
			dues:    []time.Duration{time.Minute, -90 * time.Second},
			wantErr: "d1 overdue by 1m30s",
		},
		{
			name: "a display that promised nothing is ignored",
			dues: []time.Duration{0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				now := time.Now()
				core := &fakeCore{err: tt.pingErr}
				var cos []progressor
				for i, d := range tt.dues {
					due := time.Time{}
					if d != 0 {
						due = now.Add(d)
					}
					cos = append(cos, fakeCoordinator{name: "d" + string(rune('0'+i)), due: due})
				}
				l := &liveness{core: core, coordinators: cos, log: quiet}
				err := l.check(t.Context())
				if tt.wantErr == "" {
					if err != nil {
						t.Fatalf("check = %v", err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("check = %v, want %q", err, tt.wantErr)
				}
				if tt.pingErr != nil && !errors.Is(err, tt.pingErr) {
					t.Error("error does not wrap the cause")
				}
			})
		})
	}
}

func TestLivenessRun(t *testing.T) {
	tests := []struct {
		name      string
		due       time.Duration // after the start
		cancelAt  time.Duration // after the start; 0: never
		wantPings int
		wantErr   string
	}{
		{name: "the first failure stops the run", due: 75 * time.Second, wantPings: 3, wantErr: "hall overdue by 15s"},
		{name: "cancellation ends a healthy run", due: time.Hour, cancelAt: 100 * time.Second, wantPings: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				core := &fakeCore{}
				co := fakeCoordinator{name: "hall", due: start.Add(tt.due)}
				l := &liveness{core: core, coordinators: []progressor{co}, log: quiet}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var failures []error
				done := make(chan struct{})
				go func() {
					l.run(ctx, livenessEvery, func(err error) { failures = append(failures, err) })
					close(done)
				}()
				if tt.cancelAt > 0 {
					time.Sleep(tt.cancelAt)
					cancel()
				}
				<-done
				for i, at := range core.pings {
					if want := start.Add(time.Duration(i+1) * livenessEvery); !at.Equal(want) {
						t.Errorf("ping %d at %v, want %v", i, at, want)
					}
				}
				if len(core.pings) != tt.wantPings {
					t.Errorf("%d pings, want %d", len(core.pings), tt.wantPings)
				}
				if tt.wantErr == "" {
					if len(failures) != 0 {
						t.Errorf("failures = %v", failures)
					}
					return
				}
				if len(failures) != 1 || !strings.Contains(failures[0].Error(), tt.wantErr) {
					t.Fatalf("failures = %v, want one containing %q", failures, tt.wantErr)
				}
			})
		})
	}
}

func TestShutdownGuard(t *testing.T) {
	tests := []struct {
		name     string
		disarmAt time.Duration // 0: never
		want     []int
	}{
		{name: "exits with 2 past the budget", want: []int{2}},
		{name: "disarmed in time", disarmAt: 59 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var mu sync.Mutex // orders the timer's goroutine before the reads and the restore
				var codes []int
				exit = func(code int) {
					mu.Lock()
					defer mu.Unlock()
					codes = append(codes, code)
				}
				t.Cleanup(func() { exit = os.Exit })
				disarm := shutdownGuard(time.Minute, quiet)
				if tt.disarmAt > 0 {
					time.Sleep(tt.disarmAt)
					disarm()
				}
				time.Sleep(2 * time.Minute)
				mu.Lock()
				defer mu.Unlock()
				if !slices.Equal(codes, tt.want) {
					t.Errorf("exit codes = %v, want %v", codes, tt.want)
				}
			})
		})
	}
}
