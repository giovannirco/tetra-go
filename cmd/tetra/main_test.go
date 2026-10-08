package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets the test read logs while the server goroutine writes them.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func TestRunServesAndShutsDown(t *testing.T) {
	port := freePort(t)
	env := map[string]string{"PORT": port, "DRAIN_DELAY_SECONDS": "0"}
	ctx, cancel := context.WithCancel(context.Background())
	logs := &syncBuffer{}

	done := make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }, logs) }()

	url := "http://127.0.0.1:" + port + "/api/sum?term_one=2&term_two=3"
	var resp *http.Response
	var err error
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if resp, err = http.Get(url); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("server never answered: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}

	if err := healthcheck(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("healthcheck against a running server: %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	for _, want := range []string{`"msg":"listening"`, `"msg":"shutting down"`, `"msg":"stopped"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs are missing %s:\n%s", want, logs.String())
		}
	}
}

func TestRunRejectsBadConfig(t *testing.T) {
	err := run(context.Background(), func(k string) string {
		if k == "PORT" {
			return "nope"
		}
		return ""
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "PORT") {
		t.Fatalf("err = %v, want a PORT error", err)
	}
}

func TestRunReportsListenError(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)

	err = run(context.Background(), func(k string) string {
		if k == "PORT" {
			return port
		}
		return ""
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run succeeded on a port that is in use")
	}
}

func TestHealthcheckFails(t *testing.T) {
	port := freePort(t)
	if err := healthcheck(func(k string) string {
		if k == "PORT" {
			return port
		}
		return ""
	}); err == nil {
		t.Fatal("healthcheck succeeded with nothing listening")
	}
	if err := healthcheck(func(k string) string {
		if k == "PORT" {
			return "x"
		}
		return ""
	}); err == nil {
		t.Fatal("healthcheck succeeded with a bad PORT")
	}
}
