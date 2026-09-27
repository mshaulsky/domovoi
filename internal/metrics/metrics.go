package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the counters. The zero value is not usable; use New.
type Registry struct {
	reg            *prometheus.Registry
	polls          *prometheus.CounterVec
	pollDuration   *prometheus.HistogramVec
	requests       *prometheus.CounterVec
	events         *prometheus.CounterVec
	renders        *prometheus.CounterVec
	renderDuration *prometheus.HistogramVec
	renderErrors   *prometheus.CounterVec
}

// namespace prefixes every family name.
const namespace = "domovoi"

// Histogram buckets in seconds. A poll is one or two HTTP round trips; a
// render includes the panel refresh, which takes 12–19 s on the e-ink.
var (
	pollBuckets   = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}
	renderBuckets = []float64{0.5, 1, 2.5, 5, 10, 15, 20, 30, 60}
)

// New returns a registry with every family registered, plus the standard Go
// runtime and process collectors.
func New() *Registry {
	r := &Registry{
		reg: prometheus.NewRegistry(),
		polls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "polls_total",
			Help: "Polls of a source by result.",
		}, []string{"source", "result"}),
		pollDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "poll_duration_seconds",
			Help: "Time spent polling a source.", Buckets: pollBuckets,
		}, []string{"source"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "requests_total",
			Help: "Upstream requests made by a source.",
		}, []string{"source"}),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "events_total",
			Help: "Journal events by kind.",
		}, []string{"kind"}),
		renders: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "renders_total",
			Help: "Frames shown on a display by refresh mode.",
		}, []string{"display", "mode"}),
		renderDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "render_duration_seconds",
			Help: "Time from render start to the display returning.", Buckets: renderBuckets,
		}, []string{"display"}),
		renderErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "render_errors_total",
			Help: "Frames a display failed to show.",
		}, []string{"display"}),
	}
	r.reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		r.polls, r.pollDuration, r.requests, r.events,
		r.renders, r.renderDuration, r.renderErrors,
	)
	return r
}

// IncPollSuccess counts a successful poll.
func (r *Registry) IncPollSuccess(source string) {
	r.polls.WithLabelValues(source, "success").Inc()
}

// IncPollError counts a failed poll.
func (r *Registry) IncPollError(source string) {
	r.polls.WithLabelValues(source, "error").Inc()
}

// ObservePollDuration records how long a poll took.
func (r *Registry) ObservePollDuration(source string, d time.Duration) {
	r.pollDuration.WithLabelValues(source).Observe(d.Seconds())
}

// IncRequest counts one upstream request of a source.
func (r *Registry) IncRequest(source string) {
	r.requests.WithLabelValues(source).Inc()
}

// IncEvent counts a journal event.
func (r *Registry) IncEvent(kind string) {
	r.events.WithLabelValues(kind).Inc()
}

// IncRender counts a frame shown on a display.
func (r *Registry) IncRender(display, mode string) {
	r.renders.WithLabelValues(display, mode).Inc()
}

// ObserveRenderDuration records how long a render-and-show took.
func (r *Registry) ObserveRenderDuration(display string, d time.Duration) {
	r.renderDuration.WithLabelValues(display).Observe(d.Seconds())
}

// IncRenderError counts a frame the display failed to show.
func (r *Registry) IncRenderError(display string) {
	r.renderErrors.WithLabelValues(display).Inc()
}

// Handler serves the registry in the Prometheus exposition format; httpd
// mounts it at /metrics.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
