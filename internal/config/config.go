// Package config loads Warren's runtime configuration.
//
// Warren is configured entirely through environment variables (12-factor
// style) so that the same container image runs unchanged across replicas
// and environments. No configuration files, no local state.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	// Listen is the address the HTTP server binds, e.g. ":8080".
	Listen string
	// DatabaseURL is the PostgreSQL connection string. Optional until the
	// first schema lands; serving without it marks readiness degraded.
	DatabaseURL string
	// ShutdownGrace is how long in-flight requests get to finish after
	// SIGTERM before the server exits. Keep it below the orchestrator's
	// termination grace period.
	ShutdownGrace time.Duration
}

// FromEnv builds a Config from WARREN_* environment variables, applying
// defaults for anything unset.
func FromEnv() (Config, error) {
	cfg := Config{
		Listen:        getenv("WARREN_LISTEN", ":8080"),
		DatabaseURL:   os.Getenv("WARREN_DATABASE_URL"),
		ShutdownGrace: 15 * time.Second,
	}

	if v := os.Getenv("WARREN_SHUTDOWN_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: WARREN_SHUTDOWN_GRACE: %w", err)
		}
		cfg.ShutdownGrace = d
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
