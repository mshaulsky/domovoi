package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// exposition scrapes the registry through its Handler and returns the text.
func exposition(t *testing.T, r *Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// assertLines fails unless every wanted line is present in the exposition.
func assertLines(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestNew(t *testing.T) {
	out := exposition(t, New())
	// Families with labels appear only once a series exists; the runtime
	// collectors are there from the first scrape.
	assertLines(t, out, "# TYPE go_goroutines gauge")
	if strings.Contains(out, "domovoi_") {
		t.Errorf("no domovoi series expected before the first increment:\n%s", out)
	}
}

func TestRegistryIncPollSuccess(t *testing.T) {
	r := New()
	r.IncPollSuccess("tuya")
	r.IncPollSuccess("tuya")
	assertLines(t, exposition(t, r),
		"# TYPE domovoi_polls_total counter",
		`domovoi_polls_total{result="success",source="tuya"} 2`)
}

func TestRegistryIncPollError(t *testing.T) {
	r := New()
	r.IncPollError("aqara")
	assertLines(t, exposition(t, r), `domovoi_polls_total{result="error",source="aqara"} 1`)
}

func TestRegistryObservePollDuration(t *testing.T) {
	r := New()
	r.ObservePollDuration("tuya", 1500*time.Millisecond)
	r.ObservePollDuration("tuya", 500*time.Millisecond)
	assertLines(t, exposition(t, r),
		"# TYPE domovoi_poll_duration_seconds histogram",
		`domovoi_poll_duration_seconds_bucket{source="tuya",le="1"} 1`,
		`domovoi_poll_duration_seconds_bucket{source="tuya",le="2.5"} 2`,
		`domovoi_poll_duration_seconds_sum{source="tuya"} 2`,
		`domovoi_poll_duration_seconds_count{source="tuya"} 2`)
}

func TestRegistryIncRequest(t *testing.T) {
	r := New()
	for range 3 {
		r.IncRequest("tuya")
	}
	assertLines(t, exposition(t, r), `domovoi_requests_total{source="tuya"} 3`)
}

func TestRegistryIncEvent(t *testing.T) {
	r := New()
	r.IncEvent("unlocked")
	r.IncEvent(`kind "quoted"`)
	assertLines(t, exposition(t, r),
		`domovoi_events_total{kind="unlocked"} 1`,
		`domovoi_events_total{kind="kind \"quoted\""} 1`)
}

func TestRegistryIncRender(t *testing.T) {
	r := New()
	r.IncRender("hall", "full")
	r.IncRender("hall", "partial")
	r.IncRender("hall", "full")
	assertLines(t, exposition(t, r),
		`domovoi_renders_total{display="hall",mode="full"} 2`,
		`domovoi_renders_total{display="hall",mode="partial"} 1`)
}

func TestRegistryObserveRenderDuration(t *testing.T) {
	r := New()
	r.ObserveRenderDuration("hall", 19*time.Second)
	assertLines(t, exposition(t, r),
		"# TYPE domovoi_render_duration_seconds histogram",
		`domovoi_render_duration_seconds_bucket{display="hall",le="15"} 0`,
		`domovoi_render_duration_seconds_bucket{display="hall",le="20"} 1`,
		`domovoi_render_duration_seconds_sum{display="hall"} 19`,
		`domovoi_render_duration_seconds_count{display="hall"} 1`)
}

func TestRegistryIncRenderError(t *testing.T) {
	r := New()
	r.IncRenderError("hall")
	assertLines(t, exposition(t, r), `domovoi_render_errors_total{display="hall"} 1`)
}

func TestRegistryHandler(t *testing.T) {
	r := New()
	r.IncPollSuccess("tuya")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "status", got: http.StatusText(rec.Code), want: "OK"},
		{name: "content type", got: strings.SplitN(rec.Header().Get("Content-Type"), ";", 2)[0], want: "text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
	assertLines(t, rec.Body.String(),
		"# HELP domovoi_polls_total Polls of a source by result.",
		`domovoi_polls_total{result="success",source="tuya"} 1`)
}
