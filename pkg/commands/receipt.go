package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"justixauto/pkg/events"
)

// Receipt owns immutable serialized outcome bytes. It is a candidate until the
// enclosing UnitOfWork commits; a callback returning one is not a commit proof.
type Receipt[T any] struct {
	status int
	body   []byte
}

type success[T any] struct {
	Data        T               `json:"data"`
	Revision    events.Revision `json:"revision"`
	OperationID string          `json:"operationId"`
}

// NewReceipt accepts only synchronous command/create outcomes. T is an explicit
// owner receipt DTO with affected IDs/state and related object string revisions;
// the command schema must validate its permitted fields before persistence.
func NewReceipt[T any](status int, data T, revision events.Revision, operationID string) (Receipt[T], error) {
	if (status != 200 && status != 201) || !validID(operationID) {
		return Receipt[T]{}, ErrInvalidReceipt
	}
	b, err := json.Marshal(success[T]{data, revision, operationID})
	if err != nil {
		return Receipt[T]{}, ErrInvalidReceipt
	}
	r := Receipt[T]{status, b}
	if _, err := decodeReceipt(r, func(T) error { return nil }); err != nil {
		return Receipt[T]{}, ErrInvalidReceipt
	}
	return r, nil
}

func Pending[T any](operationID string) (Receipt[T], error) {
	if !validID(operationID) {
		return Receipt[T]{}, ErrInvalidReceipt
	}
	b, _ := json.Marshal(struct {
		OperationID string `json:"operationId"`
		Status      string `json:"status"`
	}{operationID, "pending"})
	return Receipt[T]{202, b}, nil
}

func (r Receipt[T]) Status() int   { return r.status }
func (r Receipt[T]) Bytes() []byte { return bytes.Clone(r.body) }

// Data returns a fresh typed copy. Pending receipts intentionally have no data.
func (r Receipt[T]) Data() (T, error) { return decodeReceipt(r, func(T) error { return nil }) }

func decodeReceipt[T any](r Receipt[T], validate func(T) error) (T, error) {
	var zero T
	if validate == nil || !safeObject(r.body) {
		return zero, ErrInvalidReceipt
	}
	if r.status == 202 {
		var p struct {
			OperationID string `json:"operationId"`
			Status      string `json:"status"`
		}
		if strictDecode(r.body, &p) != nil || !validID(p.OperationID) || p.Status != "pending" {
			return zero, ErrInvalidReceipt
		}
		return zero, nil
	}
	if r.status != 200 && r.status != 201 {
		return zero, ErrInvalidReceipt
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(r.body, &raw) != nil || len(raw) != 3 || raw["revision"] == nil {
		return zero, ErrInvalidReceipt
	}
	if !safeObject(raw["data"]) {
		return zero, ErrInvalidReceipt
	}
	var s success[T]
	if strictDecode(r.body, &s) != nil || !validID(s.OperationID) || validate(s.Data) != nil {
		return zero, ErrInvalidReceipt
	}
	return s.Data, nil
}

func strictDecode(b []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(into); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalidReceipt
	}
	return nil
}

func validID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil && id.String() == s
}

// Defense in depth, not a PII allowlist: owner schemas must project and validate
// safe fields. This additionally rejects recognizable secret-bearing keys at
// every depth without returning any input values in errors.
func safeObject(b []byte) bool {
	var object map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&object) != nil || object == nil {
		return false
	}
	return safeValue(object)
}

func safeValue(v any) bool {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			n := strings.Map(func(r rune) rune {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					return unicode.ToLower(r)
				}
				return -1
			}, k)
			for _, fragment := range []string{"password", "secret", "credential", "token", "cookie", "authorization", "sessionhandle", "mfacode"} {
				if strings.Contains(n, fragment) {
					return false
				}
			}
			if !safeValue(child) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !safeValue(child) {
				return false
			}
		}
	}
	return true
}
