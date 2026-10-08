// Package live fans calculation events out to browsers over
// Server-Sent Events, so the UI can draw traffic as it happens.
//
// The broker never blocks a request: an event that a slow subscriber
// cannot take is dropped and counted. Each pod streams its own events;
// fan-out across replicas needs a shared bus (see docs/PLAN.md).
package live

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/giovannirco/tetra-go/internal/httpx"
)

// Broker distributes published events to the connected streams.
type Broker struct {
	maxStreams int
	heartbeat  time.Duration

	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	closed bool
	done   chan struct{}

	dropped atomic.Uint64
}

// NewBroker returns a broker that accepts up to maxStreams connections.
func NewBroker(maxStreams int) *Broker {
	return &Broker{
		maxStreams: maxStreams,
		heartbeat:  15 * time.Second,
		subs:       make(map[chan []byte]struct{}),
		done:       make(chan struct{}),
	}
}

// Publish sends v, encoded as JSON, to every subscriber that has room.
func (b *Broker) Publish(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- data:
		default:
			b.dropped.Add(1)
		}
	}
}

// Subscribers returns the number of open streams.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// Dropped returns how many events were dropped for slow subscribers.
func (b *Broker) Dropped() uint64 { return b.dropped.Load() }

// Close ends every open stream and refuses new ones. http.Server.Shutdown
// waits for active requests, so streams must be ended explicitly.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.done)
	}
}

func (b *Broker) subscribe() (chan []byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.subs) >= b.maxStreams {
		return nil, false
	}
	ch := make(chan []byte, 64)
	b.subs[ch] = struct{}{}
	return ch, true
}

func (b *Broker) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
}

// ServeHTTP streams events as text/event-stream until the client leaves
// or the broker closes.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ch, ok := b.subscribe()
	if !ok {
		httpx.WriteError(w, http.StatusServiceUnavailable, "too many live streams")
		return
	}
	defer b.unsubscribe(ch)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // the server's WriteTimeout would cut the stream

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // ask reverse proxies not to buffer
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	if rc.Flush() != nil {
		return
	}

	tick := time.NewTicker(b.heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.done:
			return
		case data := <-ch:
			fmt.Fprintf(w, "event: calc\ndata: %s\n\n", data)
		case <-tick.C:
			fmt.Fprint(w, ": keep-alive\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}
