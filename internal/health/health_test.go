package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbes(t *testing.T) {
	var p Probe
	mux := http.NewServeMux()
	p.Register(mux)

	check := func(path string, status int, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != status || strings.TrimSpace(rec.Body.String()) != body {
			t.Fatalf("%s: %d %s, want %d %s", path, rec.Code, rec.Body.String(), status, body)
		}
	}

	check("/healthz", 200, `{"status":"ok"}`)
	check("/readyz", 200, `{"status":"ready"}`)

	p.Drain()
	if p.Ready() {
		t.Fatal("Ready() after Drain()")
	}
	check("/readyz", 503, `{"status":"draining"}`)
	check("/healthz", 200, `{"status":"ok"}`)
}
