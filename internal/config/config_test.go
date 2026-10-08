package config

import (
	"log/slog"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8000 || cfg.Addr() != ":8000" || cfg.LogLevel != slog.LevelInfo ||
		cfg.DrainDelay != 5*time.Second || cfg.ShutdownTimeout != 10*time.Second || cfg.MaxLiveStreams != 100 {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"PORT": "9000", "LOG_LEVEL": "debug", "DRAIN_DELAY_SECONDS": "0",
		"SHUTDOWN_TIMEOUT_SECONDS": "30", "MAX_LIVE_STREAMS": "5",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 || cfg.LogLevel != slog.LevelDebug || cfg.DrainDelay != 0 ||
		cfg.ShutdownTimeout != 30*time.Second || cfg.MaxLiveStreams != 5 {
		t.Fatalf("overrides = %+v", cfg)
	}
}

func TestInvalid(t *testing.T) {
	for _, bad := range []map[string]string{
		{"PORT": "eighty"},
		{"PORT": "70000"},
		{"LOG_LEVEL": "loud"},
		{"DRAIN_DELAY_SECONDS": "-1"},
		{"SHUTDOWN_TIMEOUT_SECONDS": "0"},
		{"MAX_LIVE_STREAMS": "x"},
	} {
		if _, err := Load(env(bad)); err == nil {
			t.Errorf("Load(%v) succeeded, want an error", bad)
		}
	}
}
