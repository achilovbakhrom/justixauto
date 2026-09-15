package events

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestRevisionCanonicalRoundTrip(t *testing.T) {
	for _, value := range []string{"0", "1", "9007199254740993", "9223372036854775807"} {
		t.Run(value, func(t *testing.T) {
			revision, err := ParseRevision(value)
			if err != nil {
				t.Fatalf("ParseRevision: %v", err)
			}
			encoded, err := json.Marshal(revision)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if got, want := string(encoded), `"`+value+`"`; got != want {
				t.Fatalf("JSON = %s, want %s", got, want)
			}
			var decoded Revision
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded != revision {
				t.Fatalf("round trip = %v, want %v", decoded, revision)
			}
		})
	}
}

func TestRevisionRejectsNonCanonicalAndOutOfRangeValues(t *testing.T) {
	for _, value := range []string{"", "-1", "+1", "01", " 1", "1 ", "1.0", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseRevision(value); !errors.Is(err, ErrInvalidRevision) {
				t.Fatalf("ParseRevision(%q) error = %v, want ErrInvalidRevision", value, err)
			}
		})
	}

	var revision Revision
	for _, input := range []string{"1", `null`, `true`, `"01"`} {
		if err := json.Unmarshal([]byte(input), &revision); !errors.Is(err, ErrInvalidRevision) {
			t.Errorf("Unmarshal(%s) error = %v, want ErrInvalidRevision", input, err)
		}
	}
	if _, err := NewRevision(-1); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("NewRevision(-1) error = %v", err)
	}
}

func TestRevisionNextAndOverflow(t *testing.T) {
	current, _ := NewRevision(41)
	next, err := current.Next()
	if err != nil || next.Int64() != 42 {
		t.Fatalf("Next() = (%v, %v), want (42, nil)", next, err)
	}
	maximum, _ := NewRevision(math.MaxInt64)
	if _, err := maximum.Next(); !errors.Is(err, ErrRevisionOverflow) {
		t.Fatalf("maximum.Next() error = %v", err)
	}
}
