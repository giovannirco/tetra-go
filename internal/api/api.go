// Package api serves the arithmetic endpoints and the operation catalogue.
//
// It depends only on calc and on two small helper packages. Metrics, the
// live event stream and the web UI observe it through Options.OnEvent and
// can be removed without touching this file.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/giovannirco/tetra-go/internal/calc"
	"github.com/giovannirco/tetra-go/internal/httpx"
	"github.com/giovannirco/tetra-go/internal/reqid"
)

// Outcome values reported on every Event and used as a metric label.
const (
	OutcomeOK             = "ok"
	OutcomeInvalidInput   = "invalid_input"
	OutcomeDivisionByZero = "division_by_zero"
	OutcomeOverflow       = "overflow"
)

// maxEchoLen bounds how much of a client-supplied term is echoed back.
const maxEchoLen = 32

// Event describes one calculation request, successful or not.
type Event struct {
	Operation  string    `json:"operation"`
	TermOne    string    `json:"term_one"`
	TermTwo    string    `json:"term_two"`
	Result     *string   `json:"result"` // a string, so 64-bit values survive JavaScript
	Status     int       `json:"status"`
	Outcome    string    `json:"outcome"`
	Error      string    `json:"error,omitempty"`
	DurationMS float64   `json:"duration_ms"`
	RequestID  string    `json:"request_id"`
	Time       time.Time `json:"time"`
}

// Info identifies the running service in the catalogue response.
type Info struct {
	Service        string `json:"service"`
	Implementation string `json:"implementation"`
	Version        string `json:"version"`
}

// Options configures Register.
type Options struct {
	Info    Info
	OnEvent func(Event) // optional; called after every calculation
}

type resultBody struct {
	Result int64 `json:"result"`
}

type operationBody struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Label  string `json:"label"`
	Path   string `json:"path"`
}

type catalogueBody struct {
	Info
	Operations []operationBody `json:"operations"`
}

// Register adds the /api routes to mux.
func Register(mux *http.ServeMux, opts Options) {
	ops := calc.All()
	catalogue := catalogueBody{Info: opts.Info}
	for _, op := range ops {
		path := "/api/" + op.Name
		catalogue.Operations = append(catalogue.Operations, operationBody{
			Name: op.Name, Symbol: op.Symbol, Label: op.Label, Path: path,
		})
		mux.Handle("GET "+path, calculate(op, opts.OnEvent))
	}
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, catalogue)
	})
	mux.HandleFunc("/api/", fallback)
}

func calculate(op calc.Operation, onEvent func(Event)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		q := r.URL.Query()
		ev := Event{
			Operation: op.Name,
			TermOne:   truncate(q.Get("term_one")),
			TermTwo:   truncate(q.Get("term_two")),
			RequestID: reqid.FromContext(r.Context()),
			Time:      start.UTC(),
		}

		result, err := evaluate(op, q)
		switch {
		case err == nil:
			s := strconv.FormatInt(result, 10)
			ev.Result, ev.Status, ev.Outcome = &s, http.StatusOK, OutcomeOK
			httpx.WriteJSON(w, http.StatusOK, resultBody{Result: result})
		default:
			ev.Status, ev.Outcome, ev.Error = http.StatusBadRequest, outcomeOf(err), err.Error()
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		}

		if onEvent != nil {
			ev.DurationMS = float64(time.Since(start).Microseconds()) / 1000
			onEvent(ev)
		}
	})
}

// inputError is a problem with the query parameters, as opposed to an
// arithmetic error from calc.
type inputError struct{ msg string }

func (e inputError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	return inputError{msg: fmt.Sprintf(format, args...)}
}

func evaluate(op calc.Operation, q url.Values) (int64, error) {
	a, err := term(q, "term_one")
	if err != nil {
		return 0, err
	}
	b, err := term(q, "term_two")
	if err != nil {
		return 0, err
	}
	return op.Apply(a, b)
}

// term parses a base-10 integer in the int64 range, with an optional sign.
func term(q url.Values, name string) (int64, error) {
	raw := q.Get(name)
	if raw == "" {
		return 0, invalid("missing query parameter %s", name)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if errors.Is(err, strconv.ErrRange) {
		return 0, invalid("%s is outside the 64-bit integer range", name)
	}
	if err != nil {
		return 0, invalid("%s must be an integer, got %q", name, truncate(raw))
	}
	return v, nil
}

func outcomeOf(err error) string {
	switch {
	case errors.Is(err, calc.ErrDivisionByZero):
		return OutcomeDivisionByZero
	case errors.Is(err, calc.ErrOverflow):
		return OutcomeOverflow
	default:
		return OutcomeInvalidInput
	}
}

// fallback answers every /api/ path no other route claimed: 405 for a known
// operation called with the wrong method, 404 for anything else.
func fallback(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/")
	if _, ok := calc.Lookup(name); ok {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	httpx.WriteError(w, http.StatusNotFound, "not found")
}

func truncate(s string) string {
	if len(s) <= maxEchoLen {
		return s
	}
	return s[:maxEchoLen] + "…"
}
