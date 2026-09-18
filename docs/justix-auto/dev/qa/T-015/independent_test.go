package inbox_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"justixauto/pkg/events"
	"justixauto/pkg/inbox"
)

// TestQA015RedriveTicketCannotCrossOwnerStore proves that the opaque ticket is
// bound to the evidence store in which its authorized request was journaled.
// Two isolated owner-local fixtures deliberately retain the same UUID/context
// but different protected bytes. A ticket requested in the first store must be
// rejected by the second before plaintext restoration or the ordinary path.
func TestQA015RedriveTicketCannotCrossOwnerStore(t *testing.T) {
	if testing.Short() {
		t.Skip("requires disposable PostgreSQL fixtures")
	}
	ctx := context.Background()
	dbA, _ := installQuarantineAdapterSchema(t)
	dbB, _ := installQuarantineAdapterSchema(t)
	security := newFixtureSecurity()

	capture := captureInput(inbox.QuarantineDirectHandler)
	capture.ConsumerName = "qa-ticket-isolation"
	qA, err := inbox.NewQuarantine(dbA, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	qB, err := inbox.NewQuarantine(dbB, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qA.Capture(ctx, capture, []byte("fixture-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := qB.Capture(ctx, capture, []byte("fixture-b")); err != nil {
		t.Fatal(err)
	}

	authorizer := &fixtureAuthorizer{}
	rA, err := inbox.NewRedriver(dbA, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	rB, err := inbox.NewRedriver(dbB, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	request := redriveRequest(capture)
	ticket, err := rA.Request(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	_, err = rB.Execute(ctx, ticket, resultInput(), inbox.OrdinaryRedriveFunc(func(context.Context, inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
		calls.Add(1)
		return inbox.OrdinaryReceipt{}, errors.New("ordinary path must not run")
	}))
	if !errors.Is(err, inbox.ErrRedriveConflict) {
		t.Fatalf("expected ticket/store conflict, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("foreign-store ticket entered ordinary path %d time(s) before final conflict: %v", calls.Load(), err)
	}
}
