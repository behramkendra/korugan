// Package config loads Korugan's configuration from the environment.
// v0.1 is deliberately env-only: no files, no flags, no magic.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Addr is the HTTP listen address, e.g. ":8080".
	Addr string
	// DatabaseURL is a pgx-compatible PostgreSQL DSN.
	DatabaseURL string
	// MasterKeyB64 is the base64-encoded 32-byte key sealing secrets at rest.
	// Empty is valid: secret storage is disabled until one is set.
	MasterKeyB64 string
	// PollInterval is the default connector sync cadence.
	PollInterval time.Duration
	// LogLevel is one of debug|info|warn|error.
	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:         getenv("KORUGAN_ADDR", ":8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		MasterKeyB64: os.Getenv("KORUGAN_MASTER_KEY"),
		PollInterval: 5 * time.Minute,
		LogLevel:     getenv("KORUGAN_LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if v := os.Getenv("KORUGAN_POLL_INTERVAL_SECONDS"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 30 {
			return cfg, fmt.Errorf("KORUGAN_POLL_INTERVAL_SECONDS must be an integer >= 30, got %q", v)
		}
		cfg.PollInterval = time.Duration(secs) * time.Second
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
