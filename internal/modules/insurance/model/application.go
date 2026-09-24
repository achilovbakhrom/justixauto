// Package model holds insurance's GORM models. It imports internal/pkg only;
// it must never import service, repository or handler.
package model

import "time"

// Application is a seller's request to have an insurer review a sale.
type Application struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	SellerCompanyID  string `gorm:"type:uuid"`
	InsurerCompanyID string `gorm:"type:uuid"`
	DealID           string `gorm:"type:uuid"`
	Status           string
	Note             string
	Snapshot         []byte `gorm:"type:jsonb"`
	Version          int64
	CreatedBy        string `gorm:"type:uuid"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	SubmittedAt      *time.Time
	DecidedAt        *time.Time
}

func (Application) TableName() string { return "insurance.applications" }

// Message is one step in an application's review history.
type Message struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	Seq           int64  `gorm:"->"`
	ApplicationID string `gorm:"type:uuid"`
	Kind          string
	RequestID     *string `gorm:"type:uuid"`
	Note          string
	CompanyID     string `gorm:"type:uuid"`
	ActorUserID   string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (Message) TableName() string { return "insurance.messages" }
