package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidRate       = errors.New("money: invalid decimal rate")
	ErrInvalidDate       = errors.New("money: invalid effective date")
	ErrInvalidPeriod     = errors.New("money: invalid effective period")
	ErrPolicyUnresolved  = errors.New("POLICY_UNRESOLVED")
	decimalRatePattern   = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	canonicalDatePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// DecimalRate is a plain decimal string. It intentionally does not assign a
// sign range, percent, basis-point, scale, periodicity, or rounding semantics;
// those constraints belong to an approved policy. Exponent notation is never
// accepted and the lexical scale is preserved.
type DecimalRate struct{ value string }

func ParseDecimalRate(value string) (DecimalRate, error) {
	if !decimalRatePattern.MatchString(value) || value == "-0" || strings.HasPrefix(value, "-0.") && allZeroFraction(value[3:]) {
		return DecimalRate{}, ErrInvalidRate
	}
	return DecimalRate{value: value}, nil
}

func allZeroFraction(value string) bool {
	return value != "" && strings.Trim(value, "0") == ""
}

func (r DecimalRate) String() string { return r.value }

// Precision reports integer and fractional digits without imposing a policy.
func (r DecimalRate) Precision() (integerDigits, fractionalDigits int) {
	value := strings.TrimPrefix(r.value, "-")
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 0 {
		return 0, 0
	}
	if len(parts) == 1 {
		return len(parts[0]), 0
	}
	return len(parts[0]), len(parts[1])
}

func (r DecimalRate) MarshalJSON() ([]byte, error) {
	if _, err := ParseDecimalRate(r.value); err != nil {
		return nil, err
	}
	return json.Marshal(r.value)
}

func (r *DecimalRate) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return ErrInvalidRate
	}
	parsed, err := ParseDecimalRate(value)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// Date is a real Gregorian calendar date with canonical YYYY-MM-DD encoding.
type Date struct{ value string }

func ParseDate(value string) (Date, error) {
	if !canonicalDatePattern.MatchString(value) {
		return Date{}, ErrInvalidDate
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02") != value {
		return Date{}, ErrInvalidDate
	}
	return Date{value: value}, nil
}

func (d Date) String() string { return d.value }
func (d Date) IsZero() bool   { return d.value == "" }

func (d Date) Compare(other Date) int { return strings.Compare(d.value, other.value) }

func (d Date) MarshalJSON() ([]byte, error) {
	if _, err := ParseDate(d.value); err != nil {
		return nil, err
	}
	return json.Marshal(d.value)
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return ErrInvalidDate
	}
	parsed, err := ParseDate(value)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// EffectivePeriod is inclusive at both ends. A zero Through date means that
// the approved policy has no recorded end date.
type EffectivePeriod struct {
	From    Date
	Through Date
}

func NewEffectivePeriod(from Date, through Date) (EffectivePeriod, error) {
	if from.IsZero() {
		return EffectivePeriod{}, ErrInvalidPeriod
	}
	if _, err := ParseDate(from.String()); err != nil {
		return EffectivePeriod{}, ErrInvalidPeriod
	}
	if !through.IsZero() {
		if _, err := ParseDate(through.String()); err != nil || through.Compare(from) < 0 {
			return EffectivePeriod{}, ErrInvalidPeriod
		}
	}
	return EffectivePeriod{From: from, Through: through}, nil
}

func (p EffectivePeriod) Contains(date Date) bool {
	if date.IsZero() || p.From.IsZero() || date.Compare(p.From) < 0 {
		return false
	}
	return p.Through.IsZero() || date.Compare(p.Through) <= 0
}

// PolicyValue makes absence of approved policy data an explicit value. Its zero
// value is unresolved and Resolve never substitutes a demo or default value.
type PolicyValue[T any] struct {
	value   T
	id      string
	version string
	period  EffectivePeriod
	ready   bool
}

func Unresolved[T any]() PolicyValue[T] { return PolicyValue[T]{} }

func Resolved[T any](id, version string, period EffectivePeriod, value T) (PolicyValue[T], error) {
	if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) || strings.TrimSpace(version) == "" || version != strings.TrimSpace(version) {
		return PolicyValue[T]{}, fmt.Errorf("money: invalid policy identity")
	}
	if _, err := NewEffectivePeriod(period.From, period.Through); err != nil {
		return PolicyValue[T]{}, err
	}
	return PolicyValue[T]{value: value, id: id, version: version, period: period, ready: true}, nil
}

func (p PolicyValue[T]) IsResolved() bool { return p.ready }
func (p PolicyValue[T]) ID() string       { return p.id }
func (p PolicyValue[T]) Version() string  { return p.version }

func (p PolicyValue[T]) Resolve(at Date) (T, error) {
	if !p.ready || !p.period.Contains(at) {
		var zero T
		return zero, ErrPolicyUnresolved
	}
	return p.value, nil
}
