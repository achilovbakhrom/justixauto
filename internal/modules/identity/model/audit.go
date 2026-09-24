package model

import "time"

// AuditEvent is an append-only record of a sensitive change.
type AuditEvent struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Seq          int64  `gorm:"->"` // assigned by the database; defines order
	OccurredAt   time.Time
	ActorUserID  *string `gorm:"type:uuid"`
	Action       string
	ResourceType string
	ResourceID   string  `gorm:"type:uuid"`
	CompanyID    *string `gorm:"type:uuid"`
	Reason       string
	Details      []byte `gorm:"type:jsonb"`
}

func (AuditEvent) TableName() string { return "identity.audit_events" }

type AuditFilter struct {
	ResourceType string
	ResourceID   string
	ActorID      string
	Limit        int
	Offset       int
}
