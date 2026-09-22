package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/jsonx"
)

// ---- DTOs ----

type specDTO struct {
	Version       string `json:"version"`
	Make          string `json:"make"`
	Model         string `json:"model"`
	Variant       string `json:"variant"`
	Year          int    `json:"year"`
	BodyType      string `json:"bodyType"`
	ExteriorColor string `json:"exteriorColor"`
	InteriorColor string `json:"interiorColor"`
	Powertrain    string `json:"powertrain"`
	Drivetrain    string `json:"drivetrain"`
}

func toSpec(m VehicleModel, s Specification) specDTO {
	return specDTO{Version: httpx.Revision(int64(s.SpecVersion)), Make: m.Make, Model: m.Model, Variant: m.Variant,
		Year: s.Year, BodyType: s.BodyType, ExteriorColor: s.ExteriorColor, InteriorColor: s.InteriorColor,
		Powertrain: s.Powertrain, Drivetrain: s.Drivetrain}
}

type modelDTO struct {
	ID            string    `json:"id"`
	Specification specDTO   `json:"specification"`
	Versions      []specDTO `json:"versions,omitempty"`
	Revision      string    `json:"revision"`
}

func toModel(d *ModelDetail) modelDTO {
	out := modelDTO{ID: d.Model.ID, Specification: toSpec(d.Model, d.Current), Revision: httpx.Revision(d.Model.Version)}
	for _, s := range d.Versions {
		out.Versions = append(out.Versions, toSpec(d.Model, s))
	}
	return out
}

type warehouseDTO struct {
	ID        string         `json:"id"`
	BranchID  *string        `json:"branchId"`
	Name      string         `json:"name"`
	Country   jsonx.Label    `json:"country"`
	Region    *jsonx.Label   `json:"region"`
	City      string         `json:"city"`
	Address   string         `json:"address"`
	Capacity  jsonx.Quantity `json:"capacity"`
	Occupied  jsonx.Quantity `json:"occupied"`
	Free      jsonx.Quantity `json:"free"`
	Revision  string         `json:"revision"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

func toWarehouse(v *WarehouseView) warehouseDTO {
	w := v.Warehouse
	d := warehouseDTO{ID: w.ID, BranchID: w.BranchID, Name: w.Name, Country: jsonx.Label{Key: w.CountryKey, Label: w.Country},
		City: w.City, Address: w.Address, Capacity: jsonx.Quantity(w.Capacity), Occupied: jsonx.Quantity(v.Occupied),
		Free: jsonx.Quantity(v.Free()), Revision: httpx.Revision(w.Version), UpdatedAt: w.UpdatedAt}
	if w.Region != "" {
		d.Region = &jsonx.Label{Key: w.RegionKey, Label: w.Region}
	}
	return d
}

type batchDTO struct {
	ID                string         `json:"id"`
	WarehouseID       string         `json:"warehouseId"`
	ModelID           string         `json:"modelId"`
	SpecVersion       string         `json:"modelSpecificationVersion"`
	ConfirmedQuantity jsonx.Quantity `json:"confirmedQuantity"`
	IdentifiedCount   jsonx.Quantity `json:"identifiedCount"`
	UnidentifiedCount jsonx.Quantity `json:"unidentifiedCount"`
	ReceivedAt        time.Time      `json:"receivedAt"`
	Revision          string         `json:"revision"`
}

func toBatch(b *ReceiptBatch) batchDTO {
	return batchDTO{ID: b.ID, WarehouseID: b.WarehouseID, ModelID: b.ModelID, SpecVersion: httpx.Revision(int64(b.SpecVersion)),
		ConfirmedQuantity: jsonx.Quantity(b.ConfirmedQuantity), IdentifiedCount: jsonx.Quantity(b.IdentifiedCount),
		UnidentifiedCount: jsonx.Quantity(b.UnidentifiedCount), ReceivedAt: b.ReceivedAt, Revision: httpx.Revision(b.Version)}
}

type placementDTO struct {
	WarehouseID    string    `json:"warehouseId"`
	ReceiptBatchID *string   `json:"receiptBatchId"`
	PlacedAt       time.Time `json:"placedAt"`
}

type vehicleDTO struct {
	ID          string        `json:"id"`
	VIN         string        `json:"vin"`
	ModelID     string        `json:"modelId"`
	SpecVersion string        `json:"modelSpecificationVersion"`
	Placement   *placementDTO `json:"placement"` // null: outside any warehouse
	Revision    string        `json:"revision"`
}

func toVehicle(r *VehicleRow) vehicleDTO {
	d := vehicleDTO{ID: r.ID, VIN: r.VIN, ModelID: r.ModelID, SpecVersion: httpx.Revision(int64(r.SpecVersion)), Revision: httpx.Revision(r.Version)}
	if r.WarehouseID != nil {
		d.Placement = &placementDTO{WarehouseID: *r.WarehouseID, ReceiptBatchID: r.ReceiptBatchID, PlacedAt: *r.PlacedAt}
	}
	return d
}

type factDTO struct {
	Type        string          `json:"type"`
	WarehouseID *string         `json:"warehouseId"`
	ActorID     string          `json:"actorId"`
	OccurredAt  time.Time       `json:"occurredAt"`
	Reason      string          `json:"reason"`
	Details     json.RawMessage `json:"details"`
}

func mapSlice[T, D any](items []T, f func(*T) D) []D {
	out := make([]D, len(items))
	for i := range items {
		out[i] = f(&items[i])
	}
	return out
}

func receiptBody(r *ReceiptResult) map[string]any {
	units := make([]vehicleDTO, len(r.Vehicles))
	for i, u := range r.Vehicles {
		units[i] = vehicleDTO{ID: u.ID, VIN: u.VIN, ModelID: u.ModelID, SpecVersion: httpx.Revision(int64(u.SpecVersion)), Revision: httpx.Revision(u.Version)}
	}
	return map[string]any{"batch": toBatch(&r.Batch), "warehouse": toWarehouse(&r.Warehouse), "vehicles": units}
}

// ---- handlers ----

type Handler struct {
	models     *ModelService
	warehouses *WarehouseService
	receipts   *ReceiptService
	vehicles   *VehicleService
}

func (h *Handler) Routes(g *echo.Group) {
	// The model catalogue is shared by all companies.
	g.GET("/vehicle-models", h.listModels, auth.Require())
	g.GET("/vehicle-models/:id", h.getModel, auth.Require())
	g.POST("/vehicle-models", h.createModel, auth.Require(PermModelsEdit))
	g.POST("/vehicle-models/:id/specification-versions", h.addSpec, auth.Require(PermModelsEdit))

	// Everything else works inside the active company.
	c := g.Group("", auth.RequireCompany())
	c.GET("/warehouses", h.listWarehouses, auth.Require(PermRead))
	c.GET("/warehouses/:id", h.getWarehouse, auth.Require(PermRead))
	c.GET("/warehouses/:id/inventory", h.warehouseStock, auth.Require(PermRead))
	c.POST("/warehouses", h.createWarehouse, auth.Require(PermWarehousesManage))
	c.PATCH("/warehouses/:id", h.updateWarehouse, auth.Require(PermWarehousesManage))
	c.POST("/warehouses/:id/capacity-changes", h.changeCapacity, auth.Require(PermWarehousesManage))
	c.POST("/warehouses/:id/branch-attachment", h.attachBranch, auth.Require(PermWarehousesManage))
	c.POST("/warehouses/:id/receipt-batches", h.receive, auth.Require(PermReceiptsCreate))
	c.POST("/receipt-batches/:id/identifications", h.identify, auth.Require(PermReceiptsCreate))
	c.POST("/receipt-batches/:id/quantity-corrections", h.correctQuantity, auth.Require(PermReceiptsCreate))
	c.GET("/vehicle-units", h.listVehicles, auth.Require(PermRead))
	c.GET("/vehicle-units/:id", h.getVehicle, auth.Require(PermRead))
	c.POST("/vehicle-units/:id/warehouse-moves", h.move, auth.Require(PermVehiclesMove))
}

func (h *Handler) listModels(c echo.Context) error {
	f := ModelFilter{Query: c.QueryParam("q")}
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

func (h *Handler) getModel(c echo.Context) error {
	m, err := h.models.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toModel(m), m.Model.Version)
}

type specBody struct {
	Specification SpecInput `json:"specification"`
}

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

func (h *Handler) listWarehouses(c echo.Context) error {
	ws, err := h.warehouses.List(c.Request().Context(), auth.Get(c))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ws, toWarehouse), nil)
}

func (h *Handler) getWarehouse(c echo.Context) error {
	w, err := h.warehouses.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

func (h *Handler) warehouseStock(c echo.Context) error {
	s, err := h.warehouses.Stock(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, map[string]any{
		"warehouse":           toWarehouse(&s.View),
		"vehicles":            mapSlice(s.Vehicles, toVehicle),
		"unidentifiedBatches": mapSlice(s.Unidentified, toBatch),
	}, s.View.Warehouse.Version)
}

func (h *Handler) createWarehouse(c echo.Context) error {
	var in WarehouseInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toWarehouse(w), w.Warehouse.Version)
}

func (h *Handler) updateWarehouse(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in WarehouseProfileInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

func (h *Handler) changeCapacity(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Capacity jsonx.Quantity `json:"capacity"`
		Reason   string         `json:"reason"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.ChangeCapacity(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Capacity, in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

func (h *Handler) receive(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in ReceiptInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.Receive(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, receiptBody(r), r.Batch.Version)
}

func (h *Handler) identify(c echo.Context) error {
	var in IdentifyInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.Identify(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, receiptBody(r), r.Batch.Version)
}

func (h *Handler) attachBranch(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		BranchID *string `json:"branchId"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	w, err := h.warehouses.AttachBranch(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.BranchID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toWarehouse(w), w.Warehouse.Version)
}

func (h *Handler) correctQuantity(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Quantity jsonx.Quantity `json:"quantity"`
		Reason   string         `json:"reason"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.receipts.CorrectQuantity(c.Request().Context(), auth.Get(c), c.Param("id"), expected, int(in.Quantity), in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, receiptBody(r), r.Batch.Version)
}

func (h *Handler) listVehicles(c echo.Context) error {
	f := VehicleFilter{Placement: c.QueryParam("placement"), WarehouseID: c.QueryParam("warehouseId")}
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

func (h *Handler) getVehicle(c echo.Context) error {
	d, err := h.vehicles.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	history := make([]factDTO, len(d.History))
	for i, f := range d.History {
		history[i] = factDTO{Type: f.FactType, WarehouseID: f.WarehouseID, ActorID: f.ActorUserID,
			OccurredAt: f.OccurredAt, Reason: f.Reason, Details: json.RawMessage(f.Details)}
	}
	body := toVehicle(&d.Vehicle)
	return httpx.Data(c, http.StatusOK, map[string]any{
		"vehicle": body, "specification": toSpec(d.Model, d.Spec), "history": history,
	}, d.Vehicle.Version)
}

func (h *Handler) move(c echo.Context) error {
	var in MoveInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.vehicles.Move(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toVehicle(v), v.Version)
}

// ---- module ----

type Module struct {
	handler *Handler
	stock   *StockService
}

func New(db *gorm.DB, now func() time.Time, branches Branches) *Module {
	if now == nil {
		now = time.Now
	}
	d := deps{store: NewStore(db), now: now, branches: branches}
	return &Module{handler: &Handler{models: &ModelService{d}, warehouses: &WarehouseService{d},
		receipts: &ReceiptService{d}, vehicles: &VehicleService{d}}, stock: &StockService{d}}
}

// Register mounts the inventory routes under /api/v1/inventory.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/inventory")) }

// Model returns a catalogue model with its current specification (for other
// modules, through their own ports). Unknown IDs return apperr.ErrNotFound.
func (m *Module) Model(ctx context.Context, id string) (*ModelDetail, error) {
	return m.handler.models.Get(ctx, id)
}

// Stock is the reservation and hand-over service for other modules.
func (m *Module) Stock() *StockService { return m.stock }
