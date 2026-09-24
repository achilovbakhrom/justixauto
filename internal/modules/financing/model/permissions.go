// Package model holds financing's GORM models and value types. It imports
// internal/pkg only; it must never import service, repository or handler.
package model

import "justixauto/internal/pkg/auth"

const (
	PermRead     = "financing.read"
	PermPrograms = "financing.programs.manage"     // provider
	PermApply    = "financing.applications.manage" // seller
	PermAgree    = "financing.applications.agree"  // seller agrees to terms (sensitive)
	PermReview   = "financing.applications.review" // provider: take, request information
	PermDecide   = "financing.applications.decide" // provider: terms, decline (sensitive)
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermPrograms, Scope: "company", Assignable: true},
	{Key: PermApply, Scope: "company", Assignable: true},
	{Key: PermAgree, Scope: "company", RequiresMFA: true, Assignable: true},
	{Key: PermReview, Scope: "company", Assignable: true},
	{Key: PermDecide, Scope: "company", RequiresMFA: true, Assignable: true},
}
