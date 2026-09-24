package model

import (
	"time"

	"justixauto/internal/pkg/money"
)

// Schemes are the accepted payment schemes.
var Schemes = []string{"cash", "own-installment", "partner-finance"}

// Purposes lists which invoices a payment scheme uses.
var Purposes = map[string][]string{
	"cash":            {"vehicle-payment", "registration"},
	"own-installment": {"first-installment", "registration"},
	"partner-finance": {"registration"},
}

type Deal struct {
	ID                    string  `gorm:"primaryKey;type:uuid"`
	CompanyID             string  `gorm:"type:uuid"`
	BranchID              string  `gorm:"type:uuid"`
	CustomerID            string  `gorm:"type:uuid"`
	LeadID                *string `gorm:"type:uuid"`
	VehicleID             string  `gorm:"type:uuid"`
	PaymentScheme         string
	PriceMinor            string
	Currency              string
	Status                string     // reserved | delivered | cancelled
	ContractSignedOn      *time.Time `gorm:"type:date"`
	ContractReference     string
	ContractFileIDs       []byte     `gorm:"type:jsonb"`
	RegisteredOn          *time.Time `gorm:"type:date"`
	PlateNumber           string
	RegistrationReference string
	DeliveredAt           *time.Time
	StatusReason          string
	Version               int64
	CreatedBy             string `gorm:"type:uuid"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (Deal) TableName() string { return "retail.deals" }

func (d *Deal) Price() money.Money {
	return money.Money{AmountMinor: d.PriceMinor, Currency: d.Currency}
}

type Invoice struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	DealID            string `gorm:"type:uuid"`
	CompanyID         string `gorm:"type:uuid"`
	Purpose           string
	AmountMinor       string
	Currency          string
	RecipientSnapshot string
	DueDate           *time.Time `gorm:"type:date"`
	Status            string
	Version           int64
	CreatedAt         time.Time
}

func (Invoice) TableName() string { return "retail.invoices" }

type Evidence struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	InvoiceID         string `gorm:"type:uuid"`
	AmountMinor       string
	Currency          string
	PaidOn            time.Time `gorm:"type:date"`
	ExternalReference string
	AttachmentIDs     []byte `gorm:"type:jsonb"`
	Seq               int64  `gorm:"->"`
	Status            string
	DecisionReason    string
	SubmittedBy       string  `gorm:"type:uuid"`
	DecidedBy         *string `gorm:"type:uuid"`
	Version           int64
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Evidence) TableName() string { return "retail.payment_evidence" }
