package model

import "time"

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
