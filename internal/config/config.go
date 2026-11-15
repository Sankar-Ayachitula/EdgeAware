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

	ttl := 15 * time.Minute
	if v := os.Getenv("TOKEN_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid TOKEN_TTL %q: %v", v, err)
		}
		ttl = d
	}

	return Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: DatabaseURL(),
		JWTSecret:   []byte(secret),
		TokenTTL:    ttl,
	}
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
