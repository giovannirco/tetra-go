package version

import (
	"runtime"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	started := time.Date(2026, 10, 8, 12, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	info := Get(started)
	if info.Service != "tetra" || info.Implementation != "go" || info.Runtime != runtime.Version() {
		t.Fatalf("info = %+v", info)
	}
	if info.Commit == "" || info.Hostname == "" || !info.StartedAt.Equal(started) || info.StartedAt.Location() != time.UTC {
		t.Fatalf("info = %+v", info)
	}

	Commit = "abcdef1"
	defer func() { Commit = "" }()
	if got := Get(started).Commit; got != "abcdef1" {
		t.Fatalf("commit %q, want the build-time value", got)
	}
}
