package app

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovannirco/tetra-go/internal/config"
)

func newTestServer(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	a := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		a.Close()
		srv.Close()
	})
	return a, srv
}

func get(t *testing.T, srv *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestEndToEnd(t *testing.T) {
	_, srv := newTestServer(t)

	resp, body := get(t, srv, "/api/sub?term_one=4&term_two=1")
	if resp.StatusCode != 200 || strings.TrimSpace(body) != `{"result":3}` {
		t.Fatalf("spec example: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Request-Id") == "" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("missing request id or security headers: %v", resp.Header)
	}

	for path, status := range map[string]int{"/healthz": 200, "/readyz": 200, "/api": 200, "/version": 200} {
		if resp, body := get(t, srv, path); resp.StatusCode != status || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Errorf("%s: %d %s", path, resp.StatusCode, body)
		}
	}

	_, body = get(t, srv, "/version")
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil || v["implementation"] != "go" || v["hostname"] == "" {
		t.Fatalf("/version = %s (%v)", body, err)
	}

	_, body = get(t, srv, "/metrics")
	if !strings.Contains(body, `tetra_calc_operations_total{operation="sub",outcome="ok"} 1`) {
		t.Fatalf("calculation was not counted:\n%s", body)
	}
}

func TestStaticUI(t *testing.T) {
	_, srv := newTestServer(t)

	resp, body := get(t, srv, "/")
	if resp.StatusCode != 200 || !strings.Contains(body, "<canvas") || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("index: %d, cache %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	resp, _ = get(t, srv, "/vendor/three/build/three.module.min.js")
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Fatalf("vendor: %d, cache %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	if resp, _ := get(t, srv, "/nope.js"); resp.StatusCode != 404 {
		t.Fatalf("missing file: %d", resp.StatusCode)
	}
	post, err := http.Post(srv.URL+"/", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /: %d", post.StatusCode)
	}
}

func TestDrainAndLiveEvents(t *testing.T) {
	a, srv := newTestServer(t)

	stream, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	r := bufio.NewReader(stream.Body)
	if line, _ := r.ReadString('\n'); !strings.HasPrefix(line, "retry:") {
		t.Fatalf("first stream line %q", line)
	}

	get(t, srv, "/api/mul?term_one=6&term_two=7")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, `"operation":"mul"`) || !strings.Contains(line, `"result":"42"`) {
				t.Fatalf("event %s", line)
			}
			break
		}
	}

	a.Drain()
	if resp, body := get(t, srv, "/readyz"); resp.StatusCode != 503 {
		t.Fatalf("readyz after drain: %d %s", resp.StatusCode, body)
	}
}
