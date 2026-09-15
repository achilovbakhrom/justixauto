// Package commands provides owner-local command ledger adapters. Service apps
// consume their own typed ports; only adapters bind this package to SQL.
package commands

import "errors"

var (
	ErrInvalidCommand = errors.New("invalid command schema or scope")
	ErrInvalidReceipt = errors.New("invalid command receipt")
	ErrConflict       = errors.New("idempotency conflict")
	ErrCorruptReceipt = errors.New("stored command receipt failed validation")
)

// HTTPStatus is the approved public status for a changed-input/credential retry.
// Other errors require the owner's normal error mapping, never guessed success.
func HTTPStatus(err error) int {
	if errors.Is(err, ErrConflict) {
		return 409
	}
	return 0
}
