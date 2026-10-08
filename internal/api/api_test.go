package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovannirco/tetra-go/internal/reqid"
)

func newServer(onEvent func(Event)) http.Handler {
	mux := http.NewServeMux()
	Register(mux, Options{
		Info:    Info{Service: "tetra", Implementation: "go", Version: "test"},
		OnEvent: onEvent,
	})
	return reqid.Middleware(mux)
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestCalculations(t *testing.T) {
	tests := []struct {
		target string
		status int
		body   string
	}{
		{"/api/sum?term_one=4&term_two=1", 200, `{"result":5}`},
		{"/api/sub?term_one=4&term_two=1", 200, `{"result":3}`},
		{"/api/mul?term_one=4&term_two=1", 200, `{"result":4}`},
		{"/api/div?term_one=4&term_two=1", 200, `{"result":4}`},
		{"/api/div?term_one=7&term_two=2", 200, `{"result":3}`},
		{"/api/sum?term_one=-4&term_two=%2B10", 200, `{"result":6}`},
		{"/api/sum?term_one=9223372036854775807&term_two=0", 200, `{"result":9223372036854775807}`},

		{"/api/div?term_one=1&term_two=0", 400, `{"error":"division by zero"}`},
		{"/api/mul?term_one=9223372036854775807&term_two=2", 400, `{"error":"result overflows a 64-bit integer"}`},
		{"/api/sum?term_two=1", 400, `{"error":"missing query parameter term_one"}`},
		{"/api/sum?term_one=1", 400, `{"error":"missing query parameter term_two"}`},
		{"/api/sum?term_one=abc&term_two=1", 400, `{"error":"term_one must be an integer, got \"abc\""}`},
		{"/api/sum?term_one=1.5&term_two=1", 400, `{"error":"term_one must be an integer, got \"1.5\""}`},
		{"/api/sum?term_one=1&term_two=99999999999999999999", 400, `{"error":"term_two is outside the 64-bit integer range"}`},
	}

	h := newServer(nil)
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, tt.target)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d", rec.Code, tt.status)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.body {
				t.Fatalf("body %s, want %s", got, tt.body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content type %q", ct)
			}
		})
	}
}

func TestEvents(t *testing.T) {
	var got []Event
	h := newServer(func(e Event) { got = append(got, e) })

	do(t, h, http.MethodGet, "/api/sub?term_one=4&term_two=1")
	do(t, h, http.MethodGet, "/api/div?term_one=4&term_two=0")
	do(t, h, http.MethodGet, "/api/mul?term_one=x&term_two="+strings.Repeat("9", 40))
	do(t, h, http.MethodGet, "/api/sum?term_one=-9223372036854775808&term_two=-1")

	if len(got) != 4 {
		t.Fatalf("got %d events, want 4", len(got))
	}
	ok := got[0]
	if ok.Operation != "sub" || ok.Status != 200 || ok.Outcome != OutcomeOK || ok.Result == nil || *ok.Result != "3" {
		t.Errorf("success event = %+v", ok)
	}
	if ok.RequestID == "" || ok.Time.IsZero() || ok.DurationMS < 0 {
		t.Errorf("success event is missing metadata: %+v", ok)
	}
	if e := got[1]; e.Outcome != OutcomeDivisionByZero || e.Result != nil || e.Status != 400 {
		t.Errorf("division event = %+v", e)
	}
	if e := got[2]; e.Outcome != OutcomeInvalidInput || len(e.TermTwo) > maxEchoLen+len("…") {
		t.Errorf("invalid event = %+v", e)
	}
	if e := got[3]; e.Outcome != OutcomeOverflow {
		t.Errorf("overflow event = %+v", e)
	}
}

func TestCatalogue(t *testing.T) {
	rec := do(t, newServer(nil), http.MethodGet, "/api")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body catalogueBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Implementation != "go" || len(body.Operations) != 4 || body.Operations[3].Path != "/api/div" {
		t.Fatalf("catalogue = %+v", body)
	}
}

func TestFallback(t *testing.T) {
	h := newServer(nil)

	rec := do(t, h, http.MethodPost, "/api/sum?term_one=1&term_two=1")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST known op: status %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}

	rec = do(t, h, http.MethodGet, "/api/pow?term_one=2&term_two=8")
	if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != `{"error":"not found"}` {
		t.Fatalf("unknown op: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodHead, "/api/sum?term_one=1&term_two=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD: status %d", rec.Code)
	}
}
