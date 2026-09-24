package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"justixauto/internal/pkg/jsonx"
)

// PermTrade covers the deal flow: RFQs, quotations, orders and addenda.
const PermTrade = "commerce.trade"

type RFQLine struct {
	LineID   string         `json:"lineId"`
	ModelID  string         `json:"modelId"`
	Quantity jsonx.Quantity `json:"quantity"`
}

type RFQStatus string

const (
	RFQDraft       RFQStatus = "draft"
	RFQSent        RFQStatus = "sent"
	RFQNegotiating RFQStatus = "negotiating"
	RFQAccepted    RFQStatus = "accepted"
	RFQDeclined    RFQStatus = "declined"
	RFQCancelled   RFQStatus = "cancelled"
)

type RFQ struct {
	ID                string  `gorm:"primaryKey;type:uuid"`
	BuyerCompanyID    string  `gorm:"type:uuid"`
	SupplierCompanyID string  `gorm:"type:uuid"`
	OfferVersionID    *string `gorm:"type:uuid"`
	Lines             []byte  `gorm:"type:jsonb"`
	Status            RFQStatus
	StatusReason      string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (RFQ) TableName() string { return "commerce.rfqs" }

// DecodeLines decodes the RFQ's requested lines.
func (r *RFQ) DecodeLines() []RFQLine {
	var ls []RFQLine
	_ = json.Unmarshal(r.Lines, &ls)
	return ls
}

type Quotation struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	RFQID     string `gorm:"column:rfq_id;type:uuid"`
	Number    int
	Terms     []byte `gorm:"type:jsonb"`
	Digest    string
	CreatedBy string `gorm:"type:uuid"`
	CreatedAt time.Time
}

func (Quotation) TableName() string { return "commerce.quotations" }

type OrderStatus string

const (
	AwaitingSupplier OrderStatus = "awaiting-supplier"
	OrderAccepted    OrderStatus = "accepted"
	OrderFulfilling  OrderStatus = "fulfilling"
	OrderCompleted   OrderStatus = "completed"
	OrderCancelled   OrderStatus = "cancelled"
)

type Order struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	BuyerCompanyID    string `gorm:"type:uuid"`
	SupplierCompanyID string `gorm:"type:uuid"`
	Source            string
	RFQID             *string `gorm:"column:rfq_id;type:uuid"`
	QuotationID       *string `gorm:"type:uuid"`
	OfferVersionID    *string `gorm:"type:uuid"`
	Terms             []byte  `gorm:"type:jsonb"`
	Status            OrderStatus
	StatusReason      string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Order) TableName() string { return "commerce.orders" }

// DecodeTerms decodes the order's current terms.
func (o *Order) DecodeTerms() Terms {
	var t Terms
	_ = json.Unmarshal(o.Terms, &t)
	return t
}

// Party returns "buyer", "supplier" or "" for companyID.
func (o *Order) Party(companyID string) string {
	switch companyID {
	case o.BuyerCompanyID:
		return "buyer"
	case o.SupplierCompanyID:
		return "supplier"
	}
	return ""
}

type Addendum struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	OrderID           string `gorm:"type:uuid"`
	Number            int
	Terms             []byte `gorm:"type:jsonb"`
	Reason            string
	ProposedByCompany string `gorm:"type:uuid"`
	Status            string // proposed | accepted | rejected
	DecisionReason    string
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Addendum) TableName() string { return "commerce.order_addenda" }

// Digest pins exact terms: accepting a quotation requires the digest the
// buyer reviewed, so a quietly changed quotation cannot be accepted.
func Digest(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
