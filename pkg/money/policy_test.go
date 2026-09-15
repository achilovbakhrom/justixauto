package money

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustDate(t *testing.T, value string) Date {
	t.Helper()
	date, err := ParseDate(value)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", value, err)
	}
	return date
}

func TestDecimalRateCanonicalValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "1", "-0.25", "0.00", "1.20", "12.345", strings.Repeat("9", 39)} {
		rate, err := ParseDecimalRate(value)
		if err != nil || rate.String() != value {
			t.Errorf("ParseDecimalRate(%q) = %q, %v", value, rate.String(), err)
		}
		encoded, err := json.Marshal(rate)
		if err != nil {
			t.Fatalf("marshal %q: %v", value, err)
		}
		var decoded DecimalRate
		if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.String() != value {
			t.Errorf("rate JSON round trip = %q, %v", decoded.String(), err)
		}
	}
	for _, value := range []string{"", "+1", "-0", "-0.00", ".5", "1.", "00", "01", "1e2", " 1"} {
		if _, err := ParseDecimalRate(value); !errors.Is(err, ErrInvalidRate) {
			t.Errorf("ParseDecimalRate(%q) error = %v", value, err)
		}
	}
	var rate DecimalRate
	if err := json.Unmarshal([]byte(`1.25`), &rate); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("numeric JSON rate error = %v", err)
	}
	precise, _ := ParseDecimalRate("-12.500")
	if integer, fraction := precise.Precision(); integer != 2 || fraction != 3 {
		t.Fatalf("Precision = (%d, %d), want (2, 3)", integer, fraction)
	}
}

func TestDateRejectsNonCanonicalAndImpossibleDates(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"2026-9-01", "2026-02-29", "2026-13-01", "0000-01-01", "2026-01-01T00:00:00Z", " 2026-01-01"} {
		if _, err := ParseDate(value); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(%q) error = %v", value, err)
		}
	}
	leap := mustDate(t, "2028-02-29")
	encoded, _ := json.Marshal(leap)
	if string(encoded) != `"2028-02-29"` {
		t.Fatalf("date JSON = %s", encoded)
	}
}

func TestEffectivePeriodInclusiveBoundsAndOpenEnd(t *testing.T) {
	t.Parallel()
	from := mustDate(t, "2026-01-01")
	through := mustDate(t, "2026-12-31")
	period, err := NewEffectivePeriod(from, through)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-01-01", "2026-06-15", "2026-12-31"} {
		if !period.Contains(mustDate(t, date)) {
			t.Errorf("period excludes %s", date)
		}
	}
	for _, date := range []string{"2025-12-31", "2027-01-01"} {
		if period.Contains(mustDate(t, date)) {
			t.Errorf("period includes %s", date)
		}
	}
	open, err := NewEffectivePeriod(from, Date{})
	if err != nil || !open.Contains(mustDate(t, "2099-01-01")) {
		t.Fatalf("open period = %#v, %v", open, err)
	}
	if _, err := NewEffectivePeriod(through, from); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("reverse period error = %v", err)
	}
	if _, err := NewEffectivePeriod(Date{}, Date{}); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("empty period error = %v", err)
	}
}

func TestPolicyValueNeverFallsBack(t *testing.T) {
	t.Parallel()
	date := mustDate(t, "2026-06-01")
	if value, err := Unresolved[string]().Resolve(date); value != "" || !errors.Is(err, ErrPolicyUnresolved) {
		t.Fatalf("unresolved Resolve = %q, %v", value, err)
	}
	period, _ := NewEffectivePeriod(mustDate(t, "2026-01-01"), mustDate(t, "2026-12-31"))
	policy, err := Resolved("approved-policy", "7", period, "authoritative-value")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := policy.Resolve(date); err != nil || value != "authoritative-value" {
		t.Fatalf("resolved Resolve = %q, %v", value, err)
	}
	if value, err := policy.Resolve(mustDate(t, "2027-01-01")); value != "" || !errors.Is(err, ErrPolicyUnresolved) {
		t.Fatalf("expired Resolve = %q, %v", value, err)
	}
	if !policy.IsResolved() || policy.ID() != "approved-policy" || policy.Version() != "7" {
		t.Fatalf("policy identity was not preserved")
	}
	for _, identity := range [][2]string{{"", "1"}, {"demo fallback", ""}, {" policy", "1"}} {
		if _, err := Resolved(identity[0], identity[1], period, "value"); err == nil {
			t.Errorf("Resolved(%q, %q) unexpectedly succeeded", identity[0], identity[1])
		}
	}
}
