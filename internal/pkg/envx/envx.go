// Package envx reads environment variables with defaults.
package envx

import "os"

// Or returns the value of key, or fallback when it is unset or empty.
func Or(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
