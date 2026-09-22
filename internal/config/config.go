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
	// FileStorage is "s3" (production) or "local" (DocumentsDir, development).
	FileStorage  string
	DocumentsDir string
	// WebDir is the web/apps checkout with built apps (npm run build); empty = API only.
	WebDir string
	S3           S3
}

// S3 selects the private bucket for uploaded files. Credentials come from the
// standard AWS chain (AWS_ACCESS_KEY_ID/..., profile or IAM role).
type S3 struct {
	Bucket, Region, Prefix, Endpoint, SSE string
	PathStyle                             bool
}

// Load reads configuration from the environment. DATABASE_URL is required;
// everything else has a safe default.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getenv("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ShutdownTimeout: 10 * time.Second,
		CookieSecure:    getenv("COOKIE_SECURE", "true") != "false",
		FileStorage:     getenv("FILE_STORAGE", "local"),
		DocumentsDir:    getenv("DOCUMENTS_DIR", "var/documents"),
		WebDir:          os.Getenv("WEB_DIR"),
		S3: S3{Bucket: os.Getenv("S3_BUCKET"), Region: getenv("S3_REGION", "us-east-1"), Prefix: getenv("S3_PREFIX", "documents/"),
			Endpoint: os.Getenv("S3_ENDPOINT"), SSE: os.Getenv("S3_SSE"), PathStyle: os.Getenv("S3_FORCE_PATH_STYLE") == "true"},
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
	switch {
	case cfg.FileStorage == "s3" && cfg.S3.Bucket == "":
		return Config{}, fmt.Errorf("config: S3_BUCKET is required when FILE_STORAGE=s3")
	case cfg.FileStorage != "s3" && cfg.FileStorage != "local":
		return Config{}, fmt.Errorf("config: FILE_STORAGE must be s3 or local")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
