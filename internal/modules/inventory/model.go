// Package inventory owns the vehicle model catalogue, physical vehicles
// (one per globally unique VIN), warehouses, receipt batches and placements.
// Companies and users are referenced by ID only.
package inventory

import "time"

// VehicleModel is a make/model/variant; details live in immutable
// specification versions so existing vehicles never change retroactively.
type VehicleModel struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	Make               string
	Model              string
	Variant            string
	CurrentSpecVersion int
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (VehicleModel) TableName() string { return "inventory.vehicle_models" }

type Specification struct {
	ModelID       string `gorm:"primaryKey;type:uuid"`
	SpecVersion   int    `gorm:"primaryKey"`
	Year          int
	BodyType      string
	ExteriorColor string
	InteriorColor string
	Powertrain    string
	Drivetrain    string
	CreatedAt     time.Time
	CreatedBy     string `gorm:"type:uuid"`
}

func (Specification) TableName() string { return "inventory.model_specifications" }

// Warehouse belongs to a company; BranchID is set only for a branch's
// primary warehouse. Capacity is a positive whole number of vehicles.
type Warehouse struct {
	ID         string  `gorm:"primaryKey;type:uuid"`
	CompanyID  string  `gorm:"type:uuid"`
	BranchID   *string `gorm:"type:uuid"`
	Name       string
	Country    string
	CountryKey string
	Region     string
	RegionKey  string
	City       string
	Address    string
	Capacity   int
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Warehouse) TableName() string { return "inventory.warehouses" }

// ReceiptBatch is N homogeneous vehicles received together. Vehicles whose
// VIN is not entered yet count as unidentified stock: they occupy space but
// are not vehicle units and cannot be sold, reserved or shipped.
type ReceiptBatch struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	CompanyID         string `gorm:"type:uuid"`
	WarehouseID       string `gorm:"type:uuid"`
	ModelID           string `gorm:"type:uuid"`
	SpecVersion       int
	ConfirmedQuantity int
	IdentifiedCount   int
	UnidentifiedCount int
	ReceivedAt        time.Time
	CreatedBy         string `gorm:"type:uuid"`
	Version           int64
	CreatedAt         time.Time
}

func (ReceiptBatch) TableName() string { return "inventory.receipt_batches" }

// VehicleUnit is one physical vehicle. The VIN never changes.
type VehicleUnit struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	VIN                string `gorm:"column:vin"`
	ModelID            string `gorm:"type:uuid"`
	SpecVersion        int
	OwnerCompanyID     *string `gorm:"type:uuid"`
	CustodianCompanyID *string `gorm:"type:uuid"`
	Version            int64
	CreatedAt          time.Time
}

func (VehicleUnit) TableName() string { return "inventory.vehicle_units" }

// Placement says in which warehouse a vehicle is; none means outside storage.
type Placement struct {
	VehicleID      string  `gorm:"primaryKey;type:uuid"`
	WarehouseID    string  `gorm:"type:uuid"`
	ReceiptBatchID *string `gorm:"type:uuid"`
	PlacedAt       time.Time
}

func (Placement) TableName() string { return "inventory.placements" }

// Fact is an append-only history entry; there is no generic status edit.
type Fact struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	Seq            int64  `gorm:"->"`
	CompanyID      string `gorm:"type:uuid"`
	FactType       string
	VehicleID      *string `gorm:"type:uuid"`
	WarehouseID    *string `gorm:"type:uuid"`
	ReceiptBatchID *string `gorm:"type:uuid"`
	ActorUserID    string  `gorm:"type:uuid"`
	OccurredAt     time.Time
	RecordedAt     time.Time
	Reason         string
	Details        []byte `gorm:"type:jsonb"`
}

func (Fact) TableName() string { return "inventory.facts" }
