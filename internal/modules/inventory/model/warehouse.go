package model

import "time"

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

// WarehouseFilter narrows a company's warehouse listing.
type WarehouseFilter struct {
	CompanyID string
	// BranchIDs limits branch-bound warehouses (nil = all); company-wide
	// warehouses (no branch) are always included.
	BranchIDs []string
}
