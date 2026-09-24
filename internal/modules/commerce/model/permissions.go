// Package model holds commerce's GORM models, enums and value types shared
// between the service and repository layers. It imports internal/pkg only;
// it must never import service, repository or handler.
package model

import "justixauto/internal/pkg/auth"

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermPartnershipsManage, Scope: "company", Assignable: true},
	{Key: PermOffersManage, Scope: "company", Assignable: true},
	{Key: PermTrade, Scope: "company", Assignable: true},
	{Key: PermPaymentsAccept, Scope: "company", RequiresMFA: true, Assignable: true},
}
