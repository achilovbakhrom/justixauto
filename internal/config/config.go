// Package config loads runtime configuration from environment variables.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"justixauto/internal/pkg/envx"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	ShutdownTimeout time.Duration
	// ShutdownDrain: after SIGTERM keep serving this long before closing (SHUTDOWN_DRAIN, e.g. 5s).
	ShutdownDrain time.Duration
	// CookieSecure must stay true outside local HTTP development.
	CookieSecure bool
	// AllowedOrigins may send state-changing requests besides the API's own host
	// (e.g. the Vite dev servers).
	AllowedOrigins []string
	// MFAKey (32 bytes, base64 in MFA_KEY) encrypts two-factor secrets at rest.
	// Losing or changing it disables every enrolled authenticator.
	MFAKey []byte
	// MFADisabled (MFA_DISABLED=true) switches two-factor authentication off. Local development only.
	MFADisabled bool
	// FileStorage is "s3" (production) or "local" (DocumentsDir, development).
	FileStorage  string
	DocumentsDir string
	// WebDir is the web/apps checkout with built apps (npm run build); empty = API only.
	WebDir string
	// APIDocs (API_DOCS=true) serves Swagger UI and the OpenAPI spec at /api/docs.
	APIDocs bool
	S3      S3
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
		HTTPAddr:        envx.Or("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		ShutdownTimeout: 10 * time.Second,
		CookieSecure:    envx.Or("COOKIE_SECURE", "true") != "false",
		FileStorage:     envx.Or("FILE_STORAGE", "local"),
		DocumentsDir:    envx.Or("DOCUMENTS_DIR", "var/documents"),
		WebDir:          os.Getenv("WEB_DIR"),
		APIDocs:         os.Getenv("API_DOCS") == "true",
		S3: S3{
			Bucket: os.Getenv("S3_BUCKET"), Region: envx.Or("S3_REGION", "us-east-1"), Prefix: envx.Or("S3_PREFIX", "documents/"),
			Endpoint: os.Getenv("S3_ENDPOINT"), SSE: os.Getenv("S3_SSE"), PathStyle: os.Getenv("S3_FORCE_PATH_STYLE") == "true",
		},
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
	if d := os.Getenv("SHUTDOWN_DRAIN"); d != "" {
		if cfg.ShutdownDrain, err = time.ParseDuration(d); err != nil {
			return Config{}, fmt.Errorf("SHUTDOWN_DRAIN: %w", err)
		}
	}
	cfg.MFADisabled = os.Getenv("MFA_DISABLED") == "true"
	switch {
	case cfg.FileStorage == "s3" && cfg.S3.Bucket == "":
		return Config{}, fmt.Errorf("config: S3_BUCKET is required when FILE_STORAGE=s3")
	case cfg.FileStorage != "s3" && cfg.FileStorage != "local":
		return Config{}, fmt.Errorf("config: FILE_STORAGE must be s3 or local")
	}
	return cfg, nil
}
