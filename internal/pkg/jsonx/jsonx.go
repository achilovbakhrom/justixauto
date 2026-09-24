// Package jsonx holds JSON value types of the HTTP contract.
package jsonx

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Quantity is a whole number sent as a decimal string ("12"); a JSON number
// is accepted too. Validation of the range is up to the service.
type Quantity int64

func (q Quantity) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(q), 10))
}

func (q *Quantity) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return errors.New("quantity must be a whole number")
	}
	*q = Quantity(n)
	return nil
}

// Label is free text optionally chosen from a catalogue (country, region).
type Label struct {
	Key   string `json:"key,omitempty"`
	Label string `json:"label"`
}
