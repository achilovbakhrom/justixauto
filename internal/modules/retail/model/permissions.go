// Package model holds retail's GORM models, enums, filters and value types.
// It imports internal/pkg only; it must never import service, repository or
// handler.
package model

import "justixauto/internal/pkg/auth"

const (
	PermRead           = "retail.read"
	PermCRM            = "retail.crm.manage"
	PermListings       = "retail.listings.manage"
	PermDeals          = "retail.deals.manage"
	PermPaymentsAccept = "retail.payments.accept" // sensitive: needs fresh MFA
	PermDeliver        = "retail.deals.deliver"   // sensitive: needs fresh MFA
)

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermCRM, Scope: "company", Assignable: true},
	{Key: PermListings, Scope: "company", Assignable: true},
	{Key: PermDeals, Scope: "company", Assignable: true},
	{Key: PermPaymentsAccept, Scope: "company", RequiresMFA: true, Assignable: true},
	{Key: PermDeliver, Scope: "company", RequiresMFA: true, Assignable: true},
}
