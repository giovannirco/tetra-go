package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovannirco/tetra-go/internal/version"
)

type fakeLive struct{}

func (fakeLive) Subscribers() int { return 3 }
func (fakeLive) Dropped() uint64  { return 7 }

func setup(t *testing.T) (*Telemetry, http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	tel := New(logger, version.Info{Implementation: "go", Version: "test", Commit: "abc1234", Runtime: "go"}, fakeLive{})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sum", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, "short and stout")
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	mux.Handle("GET /metrics", tel.Handler())
	return tel, tel.Middleware(mux), &logs
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestMetrics(t *testing.T) {
	tel, h, _ := setup(t)
	get(h, "/api/sum?term_one=1&term_two=2")
	get(h, "/no/such/path")
	tel.CalcOutcome("sum", "ok")

	body := get(h, "/metrics").Body.String()
	for _, want := range []string{
		`tetra_http_requests_total{method="GET",route="/api/sum",status="418"} 1`,
		`tetra_http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`tetra_http_request_duration_seconds_count{method="GET",route="/api/sum"} 1`,
		`tetra_calc_operations_total{operation="sum",outcome="ok"} 1`,
		`tetra_build_info{commit="abc1234",implementation="go",runtime="go",version="test"} 1`,
		`tetra_live_subscribers 3`,
		`tetra_live_events_dropped_total 7`,
		`tetra_http_requests_in_flight 1`, // the scrape itself
		`go_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output is missing %q", want)
		}
	}
}

func TestRequestLog(t *testing.T) {
	_, h, logs := setup(t)
	get(h, "/healthz") // debug level: not written at info
	get(h, "/api/sum?term_one=1&term_two=2")

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1:\n%s", len(lines), logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["msg"] != "request" || entry["route"] != "/api/sum" || entry["status"] != float64(418) ||
		entry["bytes"] != float64(len("short and stout")) {
		t.Fatalf("log entry = %v", entry)
	}
}

func TestRecorderUnwrap(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &recorder{ResponseWriter: w}
	if rec.Unwrap() != w {
		t.Fatal("Unwrap must return the wrapped writer")
	}
	rec.WriteHeader(201)
	rec.WriteHeader(500) // ignored, as net/http does
	if rec.status != 201 {
		t.Fatalf("status %d, want the first code", rec.status)
	}
}
