package model

import (
	"encoding/json"
	"math/big"
	"time"

	"justixauto/internal/pkg/jsonx"
	"justixauto/internal/pkg/money"
)

const PermOffersManage = "commerce.offers.manage"

// ---- commercial terms (contract CommercialTerms) ----

type Line struct {
	LineID    string         `json:"lineId"`
	ModelID   string         `json:"modelId"`
	Quantity  jsonx.Quantity `json:"quantity"`
	UnitPrice money.Money    `json:"unitPrice"`
}

type Installment struct {
	ID      string      `json:"id"`
	Amount  money.Money `json:"amount"`
	DueDate string      `json:"dueDate"` // YYYY-MM-DD
}

type Terms struct {
	Lines           []Line        `json:"lines"`
	Route           string        `json:"route"` // factory | foreign-direct | in-transit | local
	DeliveryTerms   string        `json:"deliveryTerms"`
	PaymentSchedule []Installment `json:"paymentSchedule"`
	WarrantyTerms   string        `json:"warrantyTerms"`
	ServiceTerms    string        `json:"serviceTerms"`
}

// Routes are the accepted delivery routes.
var Routes = []string{"factory", "foreign-direct", "in-transit", "local"}

// Audience decides which partners see a published offer.
type Audience struct {
	Mode              string   `json:"mode"` // all-active | selected
	PartnerCompanyIDs []string `json:"partnerCompanyIds"`
}

// ---- model ----

type OfferStatus string

const (
	OfferDraft     OfferStatus = "draft"
	OfferPublished OfferStatus = "published"
	OfferWithdrawn OfferStatus = "withdrawn"
)

type Offer struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	SupplierCompanyID  string `gorm:"type:uuid"`
	Status             OfferStatus
	PublishedVersionID *string `gorm:"type:uuid"`
	StatusReason       string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Offer) TableName() string { return "commerce.offers" }

// OfferVersion is immutable once created.
type OfferVersion struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	OfferID      string `gorm:"type:uuid"`
	Number       int
	Terms        []byte `gorm:"type:jsonb"`
	AudienceMode string
	AudienceIDs  []byte `gorm:"type:jsonb"`
	CreatedBy    string `gorm:"type:uuid"`
	CreatedAt    time.Time
	PublishedAt  *time.Time
}

func (OfferVersion) TableName() string { return "commerce.offer_versions" }

// Decode decodes the stored terms and audience.
func (v *OfferVersion) Decode() (Terms, Audience) {
	var t Terms
	a := Audience{Mode: v.AudienceMode}
	_ = json.Unmarshal(v.Terms, &t)
	_ = json.Unmarshal(v.AudienceIDs, &a.PartnerCompanyIDs)
	return t, a
}

// TermsTotal recomputes the exact total of stored terms.
func TermsTotal(t Terms) money.Money {
	total, currency := new(big.Int), ""
	for _, l := range t.Lines {
		if price, ok := l.UnitPrice.Parse(); ok {
			total.Add(total, new(big.Int).Mul(price, big.NewInt(int64(l.Quantity))))
			currency = l.UnitPrice.Currency
		}
	}
	return money.Of(total, currency)
}
