package application

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// notifySocket stands in for systemd: a datagram socket in NOTIFY_SOCKET
// that collects every message sent to it.
func notifySocket(t *testing.T) func() []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notify")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	t.Setenv("NOTIFY_SOCKET", path)
	return func() []string {
		var got []string
		buf := make([]byte, 256)
		for {
			if err := conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			n, err := conn.Read(buf)
			if err != nil {
				return got
			}
			got = append(got, strings.TrimSpace(string(buf[:n])))
		}
	}
}

func TestNotifierReady(t *testing.T) {
	received := notifySocket(t)
	notifier{log: quiet}.ready()
	if got := received(); len(got) != 1 || got[0] != "READY=1" {
		t.Errorf("received %v, want [READY=1]", got)
	}
}

func TestNotifierStopping(t *testing.T) {
	received := notifySocket(t)
	notifier{log: quiet}.stopping()
	if got := received(); len(got) != 1 || got[0] != "STOPPING=1" {
		t.Errorf("received %v, want [STOPPING=1]", got)
	}
}

func TestNotifierWatchdog(t *testing.T) {
	tests := []struct {
		name  string
		usec  string
		pid   string
		pings bool
	}{
		{name: "enabled", usec: "100000", pid: strconv.Itoa(os.Getpid()), pings: true},
		{name: "another process", usec: "100000", pid: "1", pings: false},
		{name: "disabled", usec: "", pid: "", pings: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received := notifySocket(t)
			t.Setenv("WATCHDOG_USEC", tt.usec)
			t.Setenv("WATCHDOG_PID", tt.pid)
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Millisecond)
			defer cancel()
			notifier{log: quiet}.watchdog(ctx)
			got := received()
			pinged := false
			for _, msg := range got {
				if msg == "WATCHDOG=1" {
					pinged = true
				}
			}
			if pinged != tt.pings {
				t.Errorf("pinged = %t (%v), want %t", pinged, got, tt.pings)
			}
		})
	}
}
