package inventory

import "justixauto/internal/modules/inventory/model"

const (
	PermRead             = model.PermRead
	PermModelsEdit       = model.PermModelsEdit
	PermWarehousesManage = model.PermWarehousesManage
	PermReceiptsCreate   = model.PermReceiptsCreate
	PermVehiclesMove     = model.PermVehiclesMove
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions
