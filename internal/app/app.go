// Package app wires the packages into one http.Handler.
//
// Request path: request ID → security headers → telemetry → routes.
package app

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/giovannirco/tetra-go/internal/api"
	"github.com/giovannirco/tetra-go/internal/config"
	"github.com/giovannirco/tetra-go/internal/health"
	"github.com/giovannirco/tetra-go/internal/httpx"
	"github.com/giovannirco/tetra-go/internal/live"
	"github.com/giovannirco/tetra-go/internal/reqid"
	"github.com/giovannirco/tetra-go/internal/telemetry"
	"github.com/giovannirco/tetra-go/internal/version"
	"github.com/giovannirco/tetra-go/web"
)

// contentSecurityPolicy allows only same-origin scripts, styles and
// connections; the UI has no inline script and no CDN.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// App is the assembled service.
type App struct {
	handler http.Handler
	probe   *health.Probe
	broker  *live.Broker
}

// New builds the service from cfg.
func New(cfg config.Config, logger *slog.Logger) *App {
	info := version.Get(time.Now())
	probe := &health.Probe{}
	broker := live.NewBroker(cfg.MaxLiveStreams)
	tel := telemetry.New(logger, info, broker)

	mux := http.NewServeMux()
	api.Register(mux, api.Options{
		Info: api.Info{Service: info.Service, Implementation: info.Implementation, Version: info.Version},
		OnEvent: func(e api.Event) {
			tel.CalcOutcome(e.Operation, e.Outcome)
			broker.Publish(e)
		},
	})
	probe.Register(mux)
	mux.Handle("GET /metrics", tel.Handler())
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, info)
	})
	mux.Handle("GET /events", broker)
	mux.Handle("/", static(web.FS)) // no method in the pattern: it would conflict with /api/

	return &App{
		handler: reqid.Middleware(securityHeaders(tel.Middleware(mux))),
		probe:   probe,
		broker:  broker,
	}
}

// Handler returns the root handler.
func (a *App) Handler() http.Handler { return a.handler }

// Drain fails the readiness probe; call it when shutdown starts.
func (a *App) Drain() { a.probe.Drain() }

// Close ends the live streams so http.Server.Shutdown can finish.
func (a *App) Close() { a.broker.Close() }

// static serves the embedded UI. Vendored libraries are immutable per
// release and cached for a day; the app's own files are revalidated.
func static(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/vendor/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
