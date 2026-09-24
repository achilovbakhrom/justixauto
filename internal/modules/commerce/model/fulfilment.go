package model

import "time"

type Allocation struct {
	ID         string  `gorm:"primaryKey;type:uuid"`
	OrderID    string  `gorm:"type:uuid"`
	LineID     string  `gorm:"type:uuid"`
	VehicleID  string  `gorm:"type:uuid"`
	VIN        string  `gorm:"column:vin"`
	Status     string  // allocated | shipped | delivered | rejected | released
	ShipmentID *string `gorm:"type:uuid"`
	CreatedAt  time.Time
}

func (Allocation) TableName() string { return "commerce.order_allocations" }

// Counts reports whether an allocation still counts toward its line.
func (a Allocation) Counts() bool {
	return a.Status == "allocated" || a.Status == "shipped" || a.Status == "delivered"
}

type Shipment struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	OrderID   string `gorm:"type:uuid"`
	Route     string
	Status    string // in-transit | received
	Version   int64
	CreatedBy string `gorm:"type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Shipment) TableName() string { return "commerce.shipments" }

type Milestone struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	ShipmentID    string `gorm:"type:uuid"`
	MilestoneType string
	OccurredAt    time.Time
	Location      string
	Note          string
	RecordedBy    string `gorm:"type:uuid"`
	CompanyID     string `gorm:"type:uuid"`
	RecordedAt    time.Time
}

func (Milestone) TableName() string { return "commerce.shipment_milestones" }

// MilestoneTypes are the accepted shipment tracking milestones.
var MilestoneTypes = []string{"departed", "border-crossed", "customs-cleared", "arrived", "damage-reported"}
