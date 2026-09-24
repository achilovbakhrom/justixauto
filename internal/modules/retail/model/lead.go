package model

import (
	"slices"
	"time"
)

// Stages lead stages. Forward moves go one step at a time; lost needs a
// reason; won only happens when the vehicle is delivered.
var Stages = []string{"new", "contacted", "qualified", "test-drive", "negotiation"}

// Sources are the accepted lead sources: no external integration is implied.
var Sources = []string{"website", "telegram", "phone", "manual"}

// Channels are the accepted lead contact channels.
var Channels = []string{"phone", "telegram", "visit", "email", "other"}

type Lead struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	CompanyID      string `gorm:"type:uuid"`
	BranchID       string `gorm:"type:uuid"`
	CustomerID     string `gorm:"type:uuid"`
	Source         string
	Stage          string
	AssignedUserID *string `gorm:"type:uuid"`
	LostReason     string
	DealID         *string `gorm:"type:uuid"`
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Lead) TableName() string { return "retail.leads" }

// Open reports whether the lead is still being worked on.
func (l *Lead) Open() bool { return l.Stage != "won" && l.Stage != "lost" }

// Qualified leads can turn into a sale.
func (l *Lead) Qualified() bool {
	return slices.Index(Stages, l.Stage) >= slices.Index(Stages, "qualified")
}

type Contact struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	LeadID     string `gorm:"type:uuid"`
	Channel    string
	Note       string
	ActorID    string `gorm:"type:uuid"`
	OccurredAt time.Time
}

func (Contact) TableName() string { return "retail.lead_contacts" }

type Task struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	CompanyID   string  `gorm:"type:uuid"`
	CustomerID  string  `gorm:"type:uuid"`
	LeadID      *string `gorm:"type:uuid"`
	DealID      *string `gorm:"type:uuid"`
	OwnerUserID string  `gorm:"type:uuid"`
	DueAt       time.Time
	Title       string
	Status      string
	CompletedAt *time.Time
	CompletedBy *string `gorm:"type:uuid"`
	Version     int64
	CreatedAt   time.Time
}

func (Task) TableName() string { return "retail.tasks" }

// LeadFilter narrows a Leads search.
type LeadFilter struct {
	CompanyID string
	BranchIDs []string // nil = all branches
	Stage     string
	Limit     int
	Offset    int
}
