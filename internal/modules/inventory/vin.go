package inventory

import (
	"regexp"
	"strings"
)

// 17 characters, letters I, O and Q excluded. No national check-digit or
// manufacturer-code policy is inferred.
var vinPattern = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

// normalizeVIN trims and upper-cases a VIN; ok is false if it is malformed.
func normalizeVIN(raw string) (string, bool) {
	vin := strings.ToUpper(strings.TrimSpace(raw))
	return vin, vinPattern.MatchString(vin)
}
