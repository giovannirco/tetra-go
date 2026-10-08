// Package health serves the liveness and readiness probes.
//
// Liveness answers "is the process able to serve at all"; it never checks
// dependencies, so a slow dependency cannot get the pod restarted.
// Readiness turns 503 once shutdown starts, so Kubernetes takes the pod
// out of the Service endpoints before the listener closes.
package health

import (
	"net/http"
	"sync/atomic"

	"github.com/giovannirco/tetra-go/internal/httpx"
)

type statusBody struct {
	Status string `json:"status"`
}

// Probe holds the readiness state.
type Probe struct {
	draining atomic.Bool
}

// Drain marks the service as not ready. It cannot be undone.
func (p *Probe) Drain() { p.draining.Store(true) }

// Ready reports whether the service accepts new traffic.
func (p *Probe) Ready() bool { return !p.draining.Load() }

// Register adds /healthz and /readyz to mux.
func (p *Probe) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, statusBody{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !p.Ready() {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, statusBody{Status: "draining"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, statusBody{Status: "ready"})
	})
}
