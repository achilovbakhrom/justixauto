// Package config loads runtime configuration from environment variables.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	ShutdownTimeout time.Duration
	// CookieSecure must stay true outside local HTTP development.
	CookieSecure bool
	// AllowedOrigins may send state-changing requests besides the API's own host
	// (e.g. the Vite dev servers).
	AllowedOrigins []string
	// MFAKey (32 bytes, base64 in MFA_KEY) encrypts two-factor secrets at rest.
	// Losing or changing it disables every enrolled authenticator.
	MFAKey []byte
}

// Load reads configuration from the environment. DATABASE_URL is required;
// everything else has a safe default.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getenv("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ShutdownTimeout: 10 * time.Second,
		CookieSecure:    getenv("COOKIE_SECURE", "true") != "false",
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("MFA_KEY"))
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("config: MFA_KEY must be 32 random bytes, base64-encoded (openssl rand -base64 32)")
	}
	cfg.MFAKey = key
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
