package auth

import (
	"errors"
	"sync"
	"testing"
)

func TestAuthorizerDenyByDefaultMatrix(t *testing.T) {
	authorizer, err := NewAuthorizer([]Grant{
		{Caller: ServiceRetail, Action: "inventory.reservation.acquire", Purpose: "retail-sale"},
		{Caller: ServiceCommerce, Action: "inventory.reservation.acquire", Purpose: "wholesale-order"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		caller  Caller
		action  Action
		purpose Purpose
		allowed bool
	}{
		{"exact retail grant", Caller{service: ServiceRetail}, "inventory.reservation.acquire", "retail-sale", true},
		{"exact commerce grant", Caller{service: ServiceCommerce}, "inventory.reservation.acquire", "wholesale-order", true},
		{"wrong caller", Caller{service: ServiceIdentity}, "inventory.reservation.acquire", "retail-sale", false},
		{"wrong action", Caller{service: ServiceRetail}, "inventory.reservation.cancel", "retail-sale", false},
		{"wrong purpose", Caller{service: ServiceRetail}, "inventory.reservation.acquire", "wholesale-order", false},
		{"case mismatch", Caller{service: ServiceRetail}, "Inventory.Reservation.Acquire", "retail-sale", false},
		{"missing caller", Caller{}, "inventory.reservation.acquire", "retail-sale", false},
		{"missing action", Caller{service: ServiceRetail}, "", "retail-sale", false},
		{"missing purpose", Caller{service: ServiceRetail}, "inventory.reservation.acquire", "", false},
		{"wildcard is literal", Caller{service: ServiceRetail}, "*", "retail-sale", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := authorizer.Authorize(test.caller, test.action, test.purpose)
			if test.allowed && err != nil {
				t.Fatalf("expected admission, got %v", err)
			}
			if !test.allowed && !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("expected ErrUnauthorized, got %v", err)
			}
		})
	}
}

func TestAuthorizerRejectsInvalidConfiguration(t *testing.T) {
	for _, grant := range []Grant{
		{Caller: "unknown", Action: "read", Purpose: "status"},
		{Caller: ServiceRetail, Action: "", Purpose: "status"},
		{Caller: ServiceRetail, Action: " read", Purpose: "status"},
		{Caller: ServiceRetail, Action: "read", Purpose: "status "},
	} {
		if _, err := NewAuthorizer([]Grant{grant}); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("grant %#v: expected invalid configuration, got %v", grant, err)
		}
	}
}

func TestAuthorizerCopiesConfigurationAndSupportsConcurrentReads(t *testing.T) {
	grants := []Grant{{Caller: ServiceDocuments, Action: "document.version.read", Purpose: "application-review"}}
	authorizer, err := NewAuthorizer(grants)
	if err != nil {
		t.Fatal(err)
	}
	grants[0] = Grant{Caller: ServiceEdge, Action: "*", Purpose: "*"}

	caller := Caller{service: ServiceDocuments}
	const readers = 32
	var wait sync.WaitGroup
	wait.Add(readers)
	for range readers {
		go func() {
			defer wait.Done()
			for range 100 {
				if err := authorizer.Authorize(caller, "document.version.read", "application-review"); err != nil {
					t.Errorf("immutable concurrent grant rejected: %v", err)
					return
				}
			}
		}()
	}
	wait.Wait()
	if err := authorizer.Authorize(Caller{service: ServiceEdge}, "*", "*"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("caller mutated constructor input: %v", err)
	}
}

func TestNilAuthorizerDenies(t *testing.T) {
	var authorizer *Authorizer
	if err := authorizer.Authorize(Caller{service: ServiceRetail}, "read", "status"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected nil authorizer to deny, got %v", err)
	}
}
