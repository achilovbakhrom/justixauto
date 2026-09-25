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
	PermPaymentsAccept = "retail.payments.accept"
	PermDeliver        = "retail.deals.deliver"
)

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermCRM, Scope: "company", Assignable: true},
	{Key: PermListings, Scope: "company", Assignable: true},
	{Key: PermDeals, Scope: "company", Assignable: true},
	{Key: PermPaymentsAccept, Scope: "company", Assignable: true},
	{Key: PermDeliver, Scope: "company", Assignable: true},
}
