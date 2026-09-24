package model

import "time"

// Event is an append-only history entry of a retail resource.
type Event struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Seq          int64  `gorm:"->"`
	CompanyID    string `gorm:"type:uuid"`
	EventType    string
	ResourceType string
	ResourceID   string `gorm:"type:uuid"`
	ActorUserID  string `gorm:"type:uuid"`
	OccurredAt   time.Time
	Reason       string
	Details      []byte `gorm:"type:jsonb"`
}

func (Event) TableName() string { return "retail.events" }
