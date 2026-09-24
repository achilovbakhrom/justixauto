// Package validate holds small input checks shared by services. Each adds a
// field message to an apperr.Validation and returns the normalized value.
package validate

import (
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"justixauto/internal/pkg/apperr"
)

// Text trims value and checks its length in characters.
func Text(v *apperr.Validation, field, value string, minLen, maxLen int) string {
	value = strings.TrimSpace(value)
	if n := utf8.RuneCountInString(value); n < minLen || n > maxLen {
		if minLen > 0 {
			v.Add(field, "required, at most "+strconv.Itoa(maxLen)+" characters")
		} else {
			v.Add(field, "at most "+strconv.Itoa(maxLen)+" characters")
		}
	}
	return value
}

func Email(v *apperr.Validation, field, value string) string {
	value = strings.TrimSpace(value)
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value || len(value) > 254 {
		v.Add(field, "must be a valid email address")
	}
	return value
}

// Reason is the required justification of state changes.
func Reason(v *apperr.Validation, value string) string { return Text(v, "reason", value, 1, 500) }

// UniqueIDs validates and de-duplicates a list of UUIDs.
func UniqueIDs(v *apperr.Validation, field string, ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if uuid.Validate(id) != nil {
			v.Add(field, "must contain valid IDs")
			return out
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// IDs turns malformed path IDs into ErrNotFound instead of database errors.
func IDs(ids ...string) error {
	for _, id := range ids {
		if uuid.Validate(id) != nil {
			return apperr.ErrNotFound
		}
	}
	return nil
}
