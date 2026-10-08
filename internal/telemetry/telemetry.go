// Package telemetry records RED metrics (rate, errors, duration) for every
// request, counts calculation outcomes, and writes one structured log line
// per request.
//
// Metric names and labels are shared by all four Tetra implementations, so
// one Grafana dashboard and one set of alerts covers whichever is running.
package telemetry

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/giovannirco/tetra-go/internal/reqid"
	"github.com/giovannirco/tetra-go/internal/version"
)

// streamRoute is long-lived, so it is kept out of the latency histogram.
const streamRoute = "/events"

// quietRoutes are logged at debug level: probes and scrapes are frequent
// and carry no information while they succeed.
var quietRoutes = map[string]bool{"/healthz": true, "/readyz": true, "/metrics": true}

// Telemetry owns the Prometheus registry and the request logger.
type Telemetry struct {
	registry *prometheus.Registry
	logger   *slog.Logger

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
	calcOps  *prometheus.CounterVec
}

// LiveStats is the part of the live broker that telemetry reports on.
type LiveStats interface {
	Subscribers() int
	Dropped() uint64
}

// New registers the Tetra metrics plus the Go runtime and process collectors.
func New(logger *slog.Logger, info version.Info, live LiveStats) *Telemetry {
	t := &Telemetry{
		registry: prometheus.NewRegistry(),
		logger:   logger,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tetra_http_requests_total",
			Help: "HTTP requests by method, route and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "tetra_http_request_duration_seconds",
			Help:    "HTTP request latency by method and route.",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tetra_http_requests_in_flight",
			Help: "HTTP requests currently being served, live streams included.",
		}),
		calcOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tetra_calc_operations_total",
			Help: "Calculations by operation and outcome (ok, invalid_input, division_by_zero, overflow).",
		}, []string{"operation", "outcome"}),
	}

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tetra_build_info",
		Help: "Always 1; labels identify the running build.",
		ConstLabels: prometheus.Labels{
			"implementation": info.Implementation, "version": info.Version,
			"commit": info.Commit, "runtime": info.Runtime,
		},
	})
	buildInfo.Set(1)

	t.registry.MustRegister(
		t.requests, t.duration, t.inFlight, t.calcOps, buildInfo,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "tetra_live_subscribers",
			Help: "Open Server-Sent Events streams.",
		}, func() float64 { return float64(live.Subscribers()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "tetra_live_events_dropped_total",
			Help: "Live events dropped because a subscriber was too slow.",
		}, func() float64 { return float64(live.Dropped()) }),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return t
}

// Handler serves /metrics in the Prometheus text format.
func (t *Telemetry) Handler() http.Handler {
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{Registry: t.registry})
}

// CalcOutcome counts one calculation.
func (t *Telemetry) CalcOutcome(operation, outcome string) {
	t.calcOps.WithLabelValues(operation, outcome).Inc()
}

// Middleware measures and logs every request that passes through it.
func (t *Telemetry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		t.inFlight.Inc()
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			t.inFlight.Dec()
			elapsed := time.Since(start)
			route := routeOf(r)
			status := strconv.Itoa(rec.status)

			t.requests.WithLabelValues(r.Method, route, status).Inc()
			if route != streamRoute {
				t.duration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
			}

			level := slog.LevelInfo
			if quietRoutes[route] && rec.status < 400 {
				level = slog.LevelDebug
			}
			t.logger.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", route),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
				slog.String("request_id", reqid.FromContext(r.Context())),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

// routeOf returns the matched ServeMux pattern without its method, so the
// route label has a fixed set of values whatever paths clients send.
func routeOf(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	return r.Pattern
}

// recorder captures the status code and size. Unwrap lets
// http.ResponseController reach Flush and SetWriteDeadline underneath.
type recorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
