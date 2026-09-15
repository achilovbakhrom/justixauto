package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Revision is a non-negative, signed 64-bit revision or sequence value.
//
// JSON and text representations are canonical base-10 strings. Keeping the
// representation as a string at transport boundaries prevents precision loss
// in clients that cannot represent every int64 exactly.
type Revision struct {
	value int64
}

var (
	ErrInvalidRevision  = errors.New("invalid revision")
	ErrRevisionOverflow = errors.New("revision overflow")
)

// NewRevision constructs a revision from its numeric value.
func NewRevision(value int64) (Revision, error) {
	if value < 0 {
		return Revision{}, fmt.Errorf("%w: must be non-negative", ErrInvalidRevision)
	}
	return Revision{value: value}, nil
}

// ParseRevision parses a canonical non-negative base-10 string.
func ParseRevision(value string) (Revision, error) {
	if value == "" {
		return Revision{}, fmt.Errorf("%w: empty value", ErrInvalidRevision)
	}
	if value == "0" {
		return Revision{}, nil
	}
	if value[0] < '1' || value[0] > '9' {
		return Revision{}, fmt.Errorf("%w: %q is not canonical base-10", ErrInvalidRevision, value)
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return Revision{}, fmt.Errorf("%w: %q is not canonical base-10", ErrInvalidRevision, value)
		}
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return Revision{}, fmt.Errorf("%w: %q: %v", ErrInvalidRevision, value, err)
	}
	return Revision{value: parsed}, nil
}

func (r Revision) String() string { return strconv.FormatInt(r.value, 10) }

func (r Revision) Int64() int64 { return r.value }

func (r Revision) IsZero() bool { return r.value == 0 }

func (r Revision) Next() (Revision, error) {
	if r.value == int64(^uint64(0)>>1) {
		return Revision{}, ErrRevisionOverflow
	}
	return Revision{value: r.value + 1}, nil
}

func (r Revision) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

func (r *Revision) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalidRevision)
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: JSON value must be a string", ErrInvalidRevision)
	}
	parsed, err := ParseRevision(value)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

func (r Revision) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

func (r *Revision) UnmarshalText(text []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalidRevision)
	}
	parsed, err := ParseRevision(string(text))
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
