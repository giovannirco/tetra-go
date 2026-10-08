package reqid

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"generates when absent", "", false},
		{"keeps a valid id", "gw-1234.abc_DEF", true},
		{"replaces unsafe characters", "bad id\n", false},
		{"replaces an oversized id", strings.Repeat("a", 65), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = FromContext(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(Header, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get(Header)
			if got == "" || got != seen {
				t.Fatalf("response id %q, context id %q", got, seen)
			}
			if tt.keep && got != tt.incoming {
				t.Fatalf("got %q, want the incoming %q", got, tt.incoming)
			}
			if !tt.keep && (got == tt.incoming || len(got) != 32) {
				t.Fatalf("got %q, want a generated 32-char id", got)
			}
		})
	}
}

func TestFromContextEmpty(t *testing.T) {
	if id := FromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); id != "" {
		t.Fatalf("got %q, want empty", id)
	}
}
