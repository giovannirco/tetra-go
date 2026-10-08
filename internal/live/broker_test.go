package live

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// openStream connects to srv and returns a reader positioned after the
// initial retry hint.
func openStream(t *testing.T, srv *httptest.Server) (*bufio.Reader, func()) {
	t.Helper()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(resp.Body)
	readUntil(t, r, "retry: 3000")
	return r, func() { resp.Body.Close() }
}

func readUntil(t *testing.T, r *bufio.Reader, want string) {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended before %q: %v", want, err)
		}
		if strings.TrimSpace(line) == want {
			return
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPublishReachesSubscriber(t *testing.T) {
	b := NewBroker(10)
	srv := httptest.NewServer(b)
	defer srv.Close()

	r, closeStream := openStream(t, srv)
	defer closeStream()
	waitFor(t, func() bool { return b.Subscribers() == 1 })

	b.Publish(map[string]string{"operation": "sum"})
	readUntil(t, r, "event: calc")
	readUntil(t, r, `data: {"operation":"sum"}`)

	closeStream()
	waitFor(t, func() bool { return b.Subscribers() == 0 })
}

func TestHeartbeat(t *testing.T) {
	b := NewBroker(10)
	b.heartbeat = 10 * time.Millisecond
	srv := httptest.NewServer(b)
	defer srv.Close()

	r, closeStream := openStream(t, srv)
	defer closeStream()
	readUntil(t, r, ": keep-alive")
}

func TestStreamLimit(t *testing.T) {
	b := NewBroker(1)
	srv := httptest.NewServer(b)
	defer srv.Close()

	_, closeStream := openStream(t, srv)
	defer closeStream()
	waitFor(t, func() bool { return b.Subscribers() == 1 })

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second stream: status %d, want 503", resp.StatusCode)
	}
}

func TestCloseEndsStreams(t *testing.T) {
	b := NewBroker(10)
	srv := httptest.NewServer(b)
	defer srv.Close()

	r, closeStream := openStream(t, srv)
	defer closeStream()
	waitFor(t, func() bool { return b.Subscribers() == 1 })

	b.Close()
	b.Close() // idempotent
	if _, err := r.ReadString('\n'); err == nil {
		// Drain whatever is buffered; the stream must end.
		for err == nil {
			_, err = r.ReadString('\n')
		}
	}
	waitFor(t, func() bool { return b.Subscribers() == 0 })

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("after Close: status %d, want 503", resp.StatusCode)
	}
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	b := NewBroker(10)
	ch, ok := b.subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer b.unsubscribe(ch)

	for i := 0; i < cap(ch)+5; i++ {
		b.Publish(i)
	}
	if got := b.Dropped(); got != 5 {
		t.Fatalf("dropped %d, want 5", got)
	}

	b.Publish(func() {}) // not JSON-encodable: ignored, not counted
	if got := b.Dropped(); got != 5 {
		t.Fatalf("dropped %d after an unencodable value, want 5", got)
	}
}
