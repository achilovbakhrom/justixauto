// Package handler holds inventory's Echo handlers, request/response DTOs and
// their mappers. It imports service, model and internal/pkg; it must never
// import repository or gorm.
package handler

import (
	"encoding/json"
	"time"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/modules/inventory/service"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/jsonx"
)

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

func toSpec(m model.VehicleModel, s model.Specification) specDTO {
	return specDTO{
		Version: httpx.Revision(int64(s.SpecVersion)), Make: m.Make, Model: m.Model, Variant: m.Variant,
		Year: s.Year, BodyType: s.BodyType, ExteriorColor: s.ExteriorColor, InteriorColor: s.InteriorColor,
		Powertrain: s.Powertrain, Drivetrain: s.Drivetrain,
	}
}

type modelDTO struct {
	ID            string    `json:"id"`
	Specification specDTO   `json:"specification"`
	Versions      []specDTO `json:"versions,omitempty"`
	Revision      string    `json:"revision"`
}

func toModel(d *service.ModelDetail) modelDTO {
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

func toWarehouse(v *service.WarehouseView) warehouseDTO {
	w := v.Warehouse
	d := warehouseDTO{
		ID: w.ID, BranchID: w.BranchID, Name: w.Name, Country: jsonx.Label{Key: w.CountryKey, Label: w.Country},
		City: w.City, Address: w.Address, Capacity: jsonx.Quantity(w.Capacity), Occupied: jsonx.Quantity(v.Occupied),
		Free: jsonx.Quantity(v.Free()), Revision: httpx.Revision(w.Version), UpdatedAt: w.UpdatedAt,
	}
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

func toBatch(b *model.ReceiptBatch) batchDTO {
	return batchDTO{
		ID: b.ID, WarehouseID: b.WarehouseID, ModelID: b.ModelID, SpecVersion: httpx.Revision(int64(b.SpecVersion)),
		ConfirmedQuantity: jsonx.Quantity(b.ConfirmedQuantity), IdentifiedCount: jsonx.Quantity(b.IdentifiedCount),
		UnidentifiedCount: jsonx.Quantity(b.UnidentifiedCount), ReceivedAt: b.ReceivedAt, Revision: httpx.Revision(b.Version),
	}
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
	Reserved    bool          `json:"reserved"`  // held by an order or a retail sale
	Revision    string        `json:"revision"`
}

func toVehicle(r *model.VehicleRow) vehicleDTO {
	d := vehicleDTO{ID: r.ID, VIN: r.VIN, ModelID: r.ModelID, SpecVersion: httpx.Revision(int64(r.SpecVersion)), Reserved: r.Reserved, Revision: httpx.Revision(r.Version)}
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

// receiptResponse documents the body written for a receipt batch mutation.
type receiptResponse struct {
	Batch     batchDTO     `json:"batch"`
	Warehouse warehouseDTO `json:"warehouse"`
	Vehicles  []vehicleDTO `json:"vehicles"`
}

func receiptBody(r *service.ReceiptResult) receiptResponse {
	units := make([]vehicleDTO, len(r.Vehicles))
	for i, u := range r.Vehicles {
		units[i] = vehicleDTO{ID: u.ID, VIN: u.VIN, ModelID: u.ModelID, SpecVersion: httpx.Revision(int64(u.SpecVersion)), Revision: httpx.Revision(u.Version)}
	}
	return receiptResponse{Batch: toBatch(&r.Batch), Warehouse: toWarehouse(&r.Warehouse), Vehicles: units}
}

// warehouseStockResponse documents the body written for a warehouse's current inventory.
type warehouseStockResponse struct {
	Warehouse           warehouseDTO `json:"warehouse"`
	Vehicles            []vehicleDTO `json:"vehicles"`
	UnidentifiedBatches []batchDTO   `json:"unidentifiedBatches"`
}

type specBody struct {
	Specification service.SpecInput `json:"specification"`
}

type changeCapacityRequest struct {
	Capacity jsonx.Quantity `json:"capacity"`
	Reason   string         `json:"reason"`
}

type attachBranchRequest struct {
	BranchID *string `json:"branchId"`
}

type correctQuantityRequest struct {
	Quantity jsonx.Quantity `json:"quantity"`
	Reason   string         `json:"reason"`
}

// vehicleDetailResponse documents the body written for one vehicle unit.
type vehicleDetailResponse struct {
	Vehicle       vehicleDTO `json:"vehicle"`
	Specification specDTO    `json:"specification"`
	History       []factDTO  `json:"history"`
}
