// Package apperr defines the error kinds services return. The HTTP layer maps
// kinds to status codes; services never know about HTTP.
package apperr

import (
	"errors"
	"time"
)

// Kinds. Wrap them (fmt.Errorf("%w: ...")) or use New for a specific code.
var (
	ErrNotFound             = errors.New("not found")
	ErrConflict             = errors.New("conflict")
	ErrStale                = errors.New("stale revision")
	ErrPreconditionRequired = errors.New("precondition required")
	ErrUnauthenticated      = errors.New("unauthenticated")
	ErrForbidden            = errors.New("forbidden")
)

// Error carries a machine-readable code and a safe message for clients.
type Error struct {
	Kind    error
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Kind }

// New returns an error of the given kind with a specific code, e.g.
// New(ErrConflict, "last_platform_admin", "the last platform admin cannot be removed").
func New(kind error, code, message string) error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// RateLimitedError asks the client to retry after a delay.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "too many attempts" }

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

// FieldError is a shortcut for a single invalid field.
func FieldError(field, message string) error {
	var v Validation
	v.Add(field, message)
	return v.Err()
}
