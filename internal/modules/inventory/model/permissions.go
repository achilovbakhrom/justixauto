package model

import "justixauto/internal/pkg/auth"

const (
	PermRead             = "inventory.read"
	PermModelsEdit       = "inventory.models.edit"
	PermWarehousesManage = "inventory.warehouses.manage"
	PermReceiptsCreate   = "inventory.receipts.create"
	PermVehiclesMove     = "inventory.vehicles.move"
)

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermModelsEdit, Scope: "company", Assignable: true},
	{Key: PermWarehousesManage, Scope: "company", Assignable: true},
	{Key: PermReceiptsCreate, Scope: "company", Assignable: true},
	{Key: PermVehiclesMove, Scope: "company", Assignable: true},
}
