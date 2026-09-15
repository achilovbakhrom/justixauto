// Package auth provides shared authentication and authorization transport
// primitives. It does not implement user sessions or business authorization.
package auth

import (
	"errors"
	"fmt"
	"strings"
)

// Service identifies an independently deployed JustixAuto owner or the edge.
type Service string

const (
	ServiceEdge      Service = "edge"
	ServiceIdentity  Service = "identity"
	ServiceInventory Service = "inventory"
	ServiceCommerce  Service = "commerce"
	ServiceRetail    Service = "retail"
	ServiceFinancing Service = "financing"
	ServiceInsurance Service = "insurance"
	ServiceDocuments Service = "documents"
)

var (
	ErrInvalidConfiguration = errors.New("auth: invalid configuration")
	ErrUnauthorized         = errors.New("auth: caller is not authorized")
)

func (s Service) valid() bool {
	switch s {
	case ServiceEdge, ServiceIdentity, ServiceInventory, ServiceCommerce,
		ServiceRetail, ServiceFinancing, ServiceInsurance, ServiceDocuments:
		return true
	default:
		return false
	}
}

// Caller is an authenticated internal service identity. Its fields are private
// so request data cannot manufacture an authenticated caller.
type Caller struct {
	service Service
}

// Service reports the authenticated service identity.
func (c Caller) Service() Service { return c.service }

// Valid reports whether the caller contains a known internal identity.
func (c Caller) Valid() bool { return c.service.valid() }

// Action and Purpose are intentionally separate. A grant for the same action
// under one purpose never grants it for another purpose.
type Action string
type Purpose string

// Grant is one exact caller/action/purpose admission rule.
type Grant struct {
	Caller  Service
	Action  Action
	Purpose Purpose
}

type grantKey struct {
	caller  Service
	action  Action
	purpose Purpose
}

// Authorizer is an immutable deny-by-default grant set and is safe for
// concurrent use.
type Authorizer struct {
	grants map[grantKey]struct{}
}

// NewAuthorizer copies and validates the supplied grants. Empty action or
// purpose values are rejected rather than becoming wildcards.
func NewAuthorizer(grants []Grant) (*Authorizer, error) {
	copied := make(map[grantKey]struct{}, len(grants))
	for index, grant := range grants {
		if !grant.Caller.valid() || !validToken(string(grant.Action)) || !validToken(string(grant.Purpose)) {
			return nil, fmt.Errorf("%w: invalid grant at index %d", ErrInvalidConfiguration, index)
		}
		copied[grantKey{caller: grant.Caller, action: grant.Action, purpose: grant.Purpose}] = struct{}{}
	}
	return &Authorizer{grants: copied}, nil
}

// Authorize admits only an exact configured triple. There are no wildcard,
// prefix, or case-folded matches.
func (a *Authorizer) Authorize(caller Caller, action Action, purpose Purpose) error {
	if a == nil || !caller.Valid() || !validToken(string(action)) || !validToken(string(purpose)) {
		return ErrUnauthorized
	}
	if _, ok := a.grants[grantKey{caller: caller.service, action: action, purpose: purpose}]; !ok {
		return ErrUnauthorized
	}
	return nil
}

func validToken(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}
