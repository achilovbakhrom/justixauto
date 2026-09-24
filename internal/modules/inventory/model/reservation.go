package model

import "time"

// Reservation holds a vehicle for one deal. At most one hold per vehicle is
// active, whichever module (commerce order, retail deal) asks for it.
type Reservation struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	VehicleID  string `gorm:"type:uuid"`
	CompanyID  string `gorm:"type:uuid"`
	HolderType string
	HolderID   string `gorm:"type:uuid"`
	Status     string // held | released | finalized
	Reason     string
	CreatedAt  time.Time
	ClosedAt   *time.Time
}

func (Reservation) TableName() string { return "inventory.reservations" }

// Holder identifies the deal that holds vehicles.
type Holder struct {
	Type string // commerce-order | retail-deal
	ID   string
}
