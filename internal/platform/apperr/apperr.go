// Package apperr defines the error kinds services return. Handlers translate
// them to HTTP status codes; services never know about HTTP.
package apperr

import "errors"

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict means the request clashes with existing data (e.g. duplicates).
	ErrConflict = errors.New("conflict")
	// ErrStale means the client's version is outdated; it must reload first.
	ErrStale = errors.New("stale version")
)

// ValidationError carries field-level messages for invalid input.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "validation failed" }

// Validation collects field errors; Err returns nil when there are none.
type Validation struct{ fields map[string]string }

func (v *Validation) Add(field, message string) {
	if v.fields == nil {
		v.fields = map[string]string{}
	}
	if _, exists := v.fields[field]; !exists {
		v.fields[field] = message
	}
}

func (v *Validation) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: v.fields}
}
