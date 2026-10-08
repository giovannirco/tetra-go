// Package reqid assigns every request an ID, so a log line, a metric
// exemplar and a client error report can be tied to the same request.
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Header is the request and response header that carries the ID.
const Header = "X-Request-Id"

type ctxKey struct{}

// FromContext returns the request ID stored by Middleware, or "".
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Middleware keeps a well-formed incoming X-Request-Id (so an ID set by a
// gateway survives) or generates one, then echoes it on the response.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Header)
		if !valid(id) {
			id = generate()
		}
		w.Header().Set(Header, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

func generate() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// valid accepts up to 64 characters of [A-Za-z0-9._-], which keeps
// arbitrary client input out of logs and response headers.
func valid(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
