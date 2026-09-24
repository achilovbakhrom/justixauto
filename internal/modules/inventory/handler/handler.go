package handler

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/modules/inventory/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// Handler holds the services behind the inventory HTTP routes.
type Handler struct {
	models     *service.Model
	warehouses *service.Warehouse
	receipts   *service.Receipt
	vehicles   *service.Vehicle
}

// New builds the inventory handler from its services.
func New(models *service.Model, warehouses *service.Warehouse, receipts *service.Receipt, vehicles *service.Vehicle) *Handler {
	return &Handler{models: models, warehouses: warehouses, receipts: receipts, vehicles: vehicles}
}

func (h *Handler) Routes(g *echo.Group) {
	// The model catalogue is shared by all companies.
	g.GET("/vehicle-models", h.listModels, auth.Require())
	g.GET("/vehicle-models/:id", h.getModel, auth.Require())
	g.POST("/vehicle-models", h.createModel, auth.Require(model.PermModelsEdit))
	g.POST("/vehicle-models/:id/specification-versions", h.addSpec, auth.Require(model.PermModelsEdit))

	// Everything else works inside the active company.
	c := g.Group("", auth.RequireCompany())
	c.GET("/warehouses", h.listWarehouses, auth.Require(model.PermRead))
	c.GET("/warehouses/:id", h.getWarehouse, auth.Require(model.PermRead))
	c.GET("/warehouses/:id/inventory", h.warehouseStock, auth.Require(model.PermRead))
	c.POST("/warehouses", h.createWarehouse, auth.Require(model.PermWarehousesManage))
	c.PATCH("/warehouses/:id", h.updateWarehouse, auth.Require(model.PermWarehousesManage))
	c.POST("/warehouses/:id/capacity-changes", h.changeCapacity, auth.Require(model.PermWarehousesManage))
	c.POST("/warehouses/:id/branch-attachment", h.attachBranch, auth.Require(model.PermWarehousesManage))
	c.POST("/warehouses/:id/receipt-batches", h.receive, auth.Require(model.PermReceiptsCreate))
	c.POST("/receipt-batches/:id/identifications", h.identify, auth.Require(model.PermReceiptsCreate))
	c.POST("/receipt-batches/:id/quantity-corrections", h.correctQuantity, auth.Require(model.PermReceiptsCreate))
	c.GET("/vehicle-units", h.listVehicles, auth.Require(model.PermRead))
	c.GET("/vehicle-units/:id", h.getVehicle, auth.Require(model.PermRead))
	c.POST("/vehicle-units/:id/warehouse-moves", h.move, auth.Require(model.PermVehiclesMove))
}

// listModels lists the shared vehicle model catalogue.
//
//	@Summary	List vehicle models
//	@Tags		inventory/models
//	@Param		q		query		string	false	"name search"
//	@Param		limit	query		int		false	"page size"
//	@Param		offset	query		int		false	"offset"
//	@Success	200		{object}	httpx.ListEnvelope[handler.modelDTO]
//	@Failure	401,422	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-models [get]
func (h *Handler) listModels(c echo.Context) error {
	f := model.ModelFilter{Query: c.QueryParam("q")}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	models, err := h.models.List(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(models, toModel), nil)
}

// getModel returns one vehicle model with its specification history.
//
//	@Summary	Get vehicle model
//	@Tags		inventory/models
//	@Param		id		path		string	true	"model ID"
//	@Success	200		{object}	httpx.DataEnvelope[handler.modelDTO]
//	@Failure	401,404	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-models/{id} [get]
func (h *Handler) getModel(c echo.Context) error {
	m, err := h.models.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toModel(m), m.Model.Version)
}

// createModel creates a vehicle model with its initial specification.
//
//	@Summary	Create vehicle model
//	@Tags		inventory/models
//	@Security	CSRF
//	@Param		Idempotency-Key	header		string				true	"retry key"
//	@Param		body			body		handler.specBody	true	"specification"
//	@Success	201				{object}	httpx.DataEnvelope[handler.modelDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-models [post]
func (h *Handler) createModel(c echo.Context) error {
	var in specBody
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.models.Create(c.Request().Context(), auth.Get(c), in.Specification)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toModel(m), m.Model.Version)
}

// addSpec adds a new specification version to a vehicle model.
//
//	@Summary	Add model specification version
//	@Tags		inventory/models
//	@Security	CSRF
//	@Param		id						path		string				true	"model ID"
//	@Param		If-Match				header		string				true	"revision"
//	@Param		body					body		handler.specBody	true	"specification"
//	@Param		Idempotency-Key			header		string				true	"retry key"
//	@Success	201						{object}	httpx.DataEnvelope[handler.modelDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-models/{id}/specification-versions [post]
func (h *Handler) addSpec(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in specBody
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.models.AddVersion(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Specification)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toModel(m), m.Model.Version)
}

// listWarehouses lists warehouses for the active company.
//
//	@Summary	List warehouses
//	@Tags		inventory/warehouses
//	@Success	200		{object}	httpx.ListEnvelope[handler.warehouseDTO]
//	@Failure	401,403	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses [get]
func (h *Handler) listWarehouses(c echo.Context) error {
	ws, err := h.warehouses.List(c.Request().Context(), auth.Get(c))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ws, toWarehouse), nil)
}

// getWarehouse returns one warehouse.
//
//	@Summary	Get warehouse
//	@Tags		inventory/warehouses
//	@Param		id			path		string	true	"warehouse ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.warehouseDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id} [get]
func (h *Handler) getWarehouse(c echo.Context) error {
	w, err := h.warehouses.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

// warehouseStock returns a warehouse's current vehicle inventory.
//
//	@Summary	Get warehouse inventory
//	@Tags		inventory/warehouses
//	@Param		id			path		string	true	"warehouse ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.warehouseStockResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id}/inventory [get]
func (h *Handler) warehouseStock(c echo.Context) error {
	s, err := h.warehouses.Stock(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, warehouseStockResponse{
		Warehouse:           toWarehouse(&s.View),
		Vehicles:            mapSlice(s.Vehicles, toVehicle),
		UnidentifiedBatches: mapSlice(s.Unidentified, toBatch),
	}, s.View.Warehouse.Version)
}

// createWarehouse creates a warehouse for the active company.
//
//	@Summary	Create warehouse
//	@Tags		inventory/warehouses
//	@Security	CSRF
//	@Param		Idempotency-Key	header		string					true	"retry key"
//	@Param		body			body		service.WarehouseInput	true	"warehouse"
//	@Success	201				{object}	httpx.DataEnvelope[handler.warehouseDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses [post]
func (h *Handler) createWarehouse(c echo.Context) error {
	var in service.WarehouseInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toWarehouse(w), w.Warehouse.Version)
}

// updateWarehouse edits a warehouse profile.
//
//	@Summary	Update warehouse
//	@Tags		inventory/warehouses
//	@Security	CSRF
//	@Param		id						path		string							true	"warehouse ID"
//	@Param		If-Match				header		string							true	"revision"
//	@Param		body					body		service.WarehouseProfileInput	true	"warehouse"
//	@Success	200						{object}	httpx.DataEnvelope[handler.warehouseDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id} [patch]
func (h *Handler) updateWarehouse(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.WarehouseProfileInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

// changeCapacity changes a warehouse's total capacity.
//
//	@Summary	Change warehouse capacity
//	@Tags		inventory/warehouses
//	@Security	CSRF
//	@Param		id						path		string							true	"warehouse ID"
//	@Param		If-Match				header		string							true	"revision"
//	@Param		body					body		handler.changeCapacityRequest	true	"capacity change"
//	@Param		Idempotency-Key			header		string							true	"retry key"
//	@Success	200						{object}	httpx.DataEnvelope[handler.warehouseDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id}/capacity-changes [post]
func (h *Handler) changeCapacity(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in changeCapacityRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.ChangeCapacity(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Capacity, in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

// receive records a receipt batch of vehicles into a warehouse.
//
//	@Summary	Receive vehicles
//	@Tags		inventory/receipts
//	@Security	CSRF
//	@Param		id						path		string					true	"warehouse ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		service.ReceiptInput	true	"receipt"
//	@Param		Idempotency-Key			header		string					true	"retry key"
//	@Success	201						{object}	httpx.DataEnvelope[handler.receiptResponse]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id}/receipt-batches [post]
func (h *Handler) receive(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.ReceiptInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.Receive(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, receiptBody(r), r.Batch.Version)
}

// identify assigns VINs to unidentified units in a receipt batch.
//
//	@Summary	Identify received vehicles
//	@Tags		inventory/receipts
//	@Security	CSRF
//	@Param		id					path		string					true	"receipt batch ID"
//	@Param		Idempotency-Key		header		string					true	"retry key"
//	@Param		body				body		service.IdentifyInput	true	"identifications"
//	@Success	200					{object}	httpx.DataEnvelope[handler.receiptResponse]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/inventory/receipt-batches/{id}/identifications [post]
func (h *Handler) identify(c echo.Context) error {
	var in service.IdentifyInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.Identify(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, receiptBody(r), r.Batch.Version)
}

// attachBranch attaches or detaches a warehouse from a branch.
//
//	@Summary	Attach warehouse to branch
//	@Tags		inventory/warehouses
//	@Security	CSRF
//	@Param		id						path		string						true	"warehouse ID"
//	@Param		If-Match				header		string						true	"revision"
//	@Param		body					body		handler.attachBranchRequest	true	"branch attachment"
//	@Param		Idempotency-Key			header		string						true	"retry key"
//	@Success	200						{object}	httpx.DataEnvelope[handler.warehouseDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/warehouses/{id}/branch-attachment [post]
func (h *Handler) attachBranch(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in attachBranchRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.AttachBranch(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.BranchID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

// correctQuantity corrects the unidentified quantity of a receipt batch.
//
//	@Summary	Correct receipt batch quantity
//	@Tags		inventory/receipts
//	@Security	CSRF
//	@Param		id						path		string							true	"receipt batch ID"
//	@Param		If-Match				header		string							true	"revision"
//	@Param		body					body		handler.correctQuantityRequest	true	"quantity correction"
//	@Param		Idempotency-Key			header		string							true	"retry key"
//	@Success	200						{object}	httpx.DataEnvelope[handler.receiptResponse]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/inventory/receipt-batches/{id}/quantity-corrections [post]
func (h *Handler) correctQuantity(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in correctQuantityRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.CorrectQuantity(c.Request().Context(), auth.Get(c), c.Param("id"), expected, int(in.Quantity), in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, receiptBody(r), r.Batch.Version)
}

// listVehicles lists vehicle units for the active company.
//
//	@Summary	List vehicle units
//	@Tags		inventory/vehicles
//	@Param		placement	query		string	false	"placement filter"
//	@Param		warehouseId	query		string	false	"warehouse ID filter"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.vehicleDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-units [get]
func (h *Handler) listVehicles(c echo.Context) error {
	f := model.VehicleFilter{Placement: c.QueryParam("placement"), WarehouseID: c.QueryParam("warehouseId")}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	rows, err := h.vehicles.List(c.Request().Context(), auth.Get(c), f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(rows, toVehicle), nil)
}

// getVehicle returns one vehicle unit with its specification and fact history.
//
//	@Summary	Get vehicle unit
//	@Tags		inventory/vehicles
//	@Param		id			path		string	true	"vehicle unit ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.vehicleDetailResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-units/{id} [get]
func (h *Handler) getVehicle(c echo.Context) error {
	d, err := h.vehicles.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	history := make([]factDTO, len(d.History))
	for i, f := range d.History {
		history[i] = factDTO{
			Type: f.FactType, WarehouseID: f.WarehouseID, ActorID: f.ActorUserID,
			OccurredAt: f.OccurredAt, Reason: f.Reason, Details: json.RawMessage(f.Details),
		}
	}
	body := toVehicle(&d.Vehicle)
	return httpx.Data(c, http.StatusOK, vehicleDetailResponse{
		Vehicle: body, Specification: toSpec(d.Model, d.Spec), History: history,
	}, d.Vehicle.Version)
}

// move records a vehicle unit's movement between warehouses.
//
//	@Summary	Move vehicle unit
//	@Tags		inventory/vehicles
//	@Security	CSRF
//	@Param		id					path		string				true	"vehicle unit ID"
//	@Param		Idempotency-Key		header		string				true	"retry key"
//	@Param		body				body		service.MoveInput	true	"move"
//	@Success	200					{object}	httpx.DataEnvelope[handler.vehicleDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/inventory/vehicle-units/{id}/warehouse-moves [post]
func (h *Handler) move(c echo.Context) error {
	var in service.MoveInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.vehicles.Move(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toVehicle(v), v.Version)
}
