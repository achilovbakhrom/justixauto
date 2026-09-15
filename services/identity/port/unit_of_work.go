// Package port defines Identity's infrastructure-free application boundaries.
package port

import "context"

// UnitOfWork runs one local transaction over the feature's concrete port set U.
// U must be an owner-defined struct/interface of typed repositories, never a
// database handle, arbitrary payload dispatcher or service locator. Factories
// bind every repository to the same transaction. U must not escape Run.
// Return every repository error; results are candidates until Run succeeds.
// Remote calls and broker publication belong outside this boundary. An unknown
// commit outcome requires authoritative receipt reconciliation, not blind retry.
type UnitOfWork[U any] interface {
	Run(context.Context, func(U) error) error
}
