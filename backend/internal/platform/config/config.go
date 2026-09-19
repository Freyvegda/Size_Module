// Package config loads runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Version is overridable at build time:
//
//	go build -ldflags "-X github.com/size-module/backend/internal/platform/config.Version=1.2.3"
var Version = "0.1.0-dev"

type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// DatabaseURL is a libpq style connection string. The API runs with
	// reduced functionality (optimizer only) when the database is unreachable.
	DatabaseURL string
	Env         string
	// CORSOrigins is a comma separated list of allowed browser origins.
	CORSOrigins []string
	// Workers is the number of asynchronous job workers in this process.
	Workers int
}

func Load() Config {
	return Config{
		Addr:        env("CUTOPTICS_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://cutoptics:cutoptics@localhost:5433/cutoptics?sslmode=disable"),
		Env:         env("CUTOPTICS_ENV", "dev"),
		CORSOrigins: splitAndTrim(env("CUTOPTICS_CORS_ORIGINS", "http://localhost:5173")),
		Workers:     envInt("CUTOPTICS_WORKERS", 2),
	}
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
