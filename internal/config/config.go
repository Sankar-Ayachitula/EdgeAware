// Package config reads all runtime settings from environment variables.
package config

import (
	"fmt"
	"log"
	"os"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   []byte
	TokenTTL    time.Duration

	// Cleanup of old sync bookkeeping (see package cleanup). CleanupInterval 0 disables
	// the in-process timer, e.g. when running `go run ./cmd/cleanup` from cron instead.
	CleanupInterval time.Duration
	FeedRetention   time.Duration
	OpsRetention    time.Duration
}

// Load reads configuration from the environment. DATABASE_URL wins if set;
// otherwise it is built from DB_HOST / DB_PORT / DB_USER / DB_PASSWORD / DB_NAME.
func Load() Config {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Allowed for local development only; never deploy without JWT_SECRET.
		log.Println("WARNING: JWT_SECRET not set, using an insecure development secret")
		secret = "dev-only-insecure-secret"
	}

	return Config{
		Port:            getenv("PORT", "8080"),
		DatabaseURL:     DatabaseURL(),
		JWTSecret:       []byte(secret),
		TokenTTL:        Duration("TOKEN_TTL", 15*time.Minute),
		CleanupInterval: Duration("CLEANUP_INTERVAL", 24*time.Hour),
		FeedRetention:   Duration("FEED_RETENTION", 30*24*time.Hour),
		OpsRetention:    Duration("OPS_RETENTION", 7*24*time.Hour),
	}
}

// Duration reads a Go duration such as "15m" or "720h" from the environment.
func Duration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		log.Fatalf("invalid %s %q: must be a duration like 15m or 720h", key, v)
	}
	return d
}

// DatabaseURL returns the Postgres connection string from the environment.
func DatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_USER", "postgres"),
		getenv("DB_PASSWORD", "postgres"),
		getenv("DB_NAME", "edgeaware"),
	)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
