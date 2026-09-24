package model

import "time"

// PermPaymentsAccept decides on payment evidence: a sensitive action that
// needs a recent second factor.
const PermPaymentsAccept = "commerce.payments.accept"

type Invoice struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	OrderID           string `gorm:"type:uuid"`
	SupplierCompanyID string `gorm:"type:uuid"`
	BuyerCompanyID    string `gorm:"type:uuid"`
	TotalMinor        string
	Currency          string
	Schedule          []byte `gorm:"type:jsonb"`
	Status            string // issued | void
	VoidReason        string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Invoice) TableName() string { return "commerce.invoices" }

type Evidence struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	InvoiceID         string `gorm:"type:uuid"`
	AmountMinor       string
	Currency          string
	PaidOn            time.Time `gorm:"type:date"`
	ExternalReference string
	AttachmentIDs     []byte `gorm:"type:jsonb"`
	Seq               int64  `gorm:"->"`
	Status            string // submitted | accepted | rejected
	DecisionReason    string
	SubmittedBy       string  `gorm:"type:uuid"`
	DecidedBy         *string `gorm:"type:uuid"`
	Version           int64
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Evidence) TableName() string { return "commerce.payment_evidence" }
