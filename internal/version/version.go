// Package version reports what build is running and where.
package version

import (
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

// Set at build time with -ldflags "-X .../version.Version=v1.2.3 ...".
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

// Info is the body of GET /version.
type Info struct {
	Service        string    `json:"service"`
	Implementation string    `json:"implementation"`
	Version        string    `json:"version"`
	Commit         string    `json:"commit"`
	BuildDate      string    `json:"build_date"`
	Runtime        string    `json:"runtime"`
	Hostname       string    `json:"hostname"` // the pod name in Kubernetes
	StartedAt      time.Time `json:"started_at"`
}

// Get returns the build information, filling the commit from the Go
// toolchain's VCS stamp when it was not set at build time.
func Get(startedAt time.Time) Info {
	commit := Commit
	if commit == "" {
		commit = vcsRevision()
	}
	host, _ := os.Hostname()
	return Info{
		Service:        "tetra",
		Implementation: "go",
		Version:        Version,
		Commit:         commit,
		BuildDate:      BuildDate,
		Runtime:        runtime.Version(),
		Hostname:       host,
		StartedAt:      startedAt.UTC(),
	}
}

func vcsRevision() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return "unknown"
}
