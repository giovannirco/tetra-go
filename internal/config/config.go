// Package config reads the service settings from the environment.
package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime setting. All of them have working defaults.
type Config struct {
	Port            int           // PORT, default 8000
	LogLevel        slog.Level    // LOG_LEVEL: debug, info, warn, error
	DrainDelay      time.Duration // DRAIN_DELAY_SECONDS, default 5
	ShutdownTimeout time.Duration // SHUTDOWN_TIMEOUT_SECONDS, default 10
	MaxLiveStreams  int           // MAX_LIVE_STREAMS, default 100
}

// Load builds a Config from getenv (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:            8000,
		LogLevel:        slog.LevelInfo,
		DrainDelay:      5 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		MaxLiveStreams:  100,
	}
	var err error
	if cfg.Port, err = intVar(getenv, "PORT", cfg.Port, 1, 65535); err != nil {
		return cfg, err
	}
	if cfg.MaxLiveStreams, err = intVar(getenv, "MAX_LIVE_STREAMS", cfg.MaxLiveStreams, 0, 10000); err != nil {
		return cfg, err
	}
	drain, err := intVar(getenv, "DRAIN_DELAY_SECONDS", int(cfg.DrainDelay/time.Second), 0, 300)
	if err != nil {
		return cfg, err
	}
	cfg.DrainDelay = time.Duration(drain) * time.Second
	shutdown, err := intVar(getenv, "SHUTDOWN_TIMEOUT_SECONDS", int(cfg.ShutdownTimeout/time.Second), 1, 300)
	if err != nil {
		return cfg, err
	}
	cfg.ShutdownTimeout = time.Duration(shutdown) * time.Second

	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(strings.ToUpper(v))); err != nil {
			return cfg, fmt.Errorf("LOG_LEVEL: %q is not one of debug, info, warn, error", v)
		}
	}
	return cfg, nil
}

// Addr is the listen address.
func (c Config) Addr() string { return ":" + strconv.Itoa(c.Port) }

func intVar(getenv func(string) string, name string, def, min, max int) (int, error) {
	v := getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return def, fmt.Errorf("%s: %q must be an integer between %d and %d", name, v, min, max)
	}
	return n, nil
}
