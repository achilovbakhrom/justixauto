package model

import "justixauto/internal/pkg/auth"

const (
	PermRead   = "insurance.read"
	PermApply  = "insurance.applications.manage" // seller side
	PermReview = "insurance.applications.review" // insurer: take, request information
	PermDecide = "insurance.applications.decide" // insurer: approve/decline (sensitive)
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermApply, Scope: "company", Assignable: true},
	{Key: PermReview, Scope: "company", Assignable: true},
	{Key: PermDecide, Scope: "company", RequiresMFA: true, Assignable: true},
}
