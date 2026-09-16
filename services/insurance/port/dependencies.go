package port

import "context"

// Readiness checks the configured owner's compatible persistence installation.
// It never migrates, seeds, starts a worker or grants business authorization.
type Readiness interface {
	Check(context.Context) error
}

// Dependencies is explicitly injected into a Insurance feature. U names only
// that feature's implemented ports; absent auth/business modules are not
// predeclared. Technical handles remain in the outer adapter/composition root.
type Dependencies[U any] struct {
	Transactions UnitOfWork[U]
	Persistence  Readiness
}
