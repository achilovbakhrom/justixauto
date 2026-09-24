package model

import (
	"encoding/json"
	"time"

	"justixauto/internal/pkg/apperr"
)

type Program struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	ProviderCompanyID string `gorm:"type:uuid"`
	Status            string
	PublishedVersion  *int
	StatusReason      string
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Program) TableName() string { return "financing.programs" }

type ProgramVersion struct {
	ProgramID     string `gorm:"primaryKey;type:uuid"`
	Number        int    `gorm:"primaryKey"`
	Name          string
	Currency      string
	Terms         []byte `gorm:"type:jsonb"`
	Eligibility   []byte `gorm:"type:jsonb"`
	PolicyID      string
	PolicyVersion int
	CreatedBy     string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (ProgramVersion) TableName() string { return "financing.program_versions" }

// Decode unmarshals the stored terms and eligibility of a program version.
func (v *ProgramVersion) Decode() (ProgramTerms, Eligibility) {
	var t ProgramTerms
	var e Eligibility
	_ = json.Unmarshal(v.Terms, &t)
	_ = json.Unmarshal(v.Eligibility, &e)
	return t, e
}

// ProgramTerms of the fixed-markup policy.
type ProgramTerms struct {
	MarkupBps         int   `json:"markupBps"`         // markup on the price, in basis points
	MinDownPaymentBps int   `json:"minDownPaymentBps"` // minimum down payment, from the price
	TermMonths        []int `json:"termMonths"`        // offered terms
}

// Validate checks the program terms are within acceptable bounds.
func (t ProgramTerms) Validate(v *apperr.Validation) {
	if t.MarkupBps < 0 || t.MarkupBps > 100_000 {
		v.Add("terms.markupBps", "0-100000 basis points")
	}
	if t.MinDownPaymentBps < 0 || t.MinDownPaymentBps >= 10_000 {
		v.Add("terms.minDownPaymentBps", "0-9999 basis points")
	}
	if len(t.TermMonths) == 0 || len(t.TermMonths) > 20 {
		v.Add("terms.termMonths", "offer 1-20 terms")
	}
	for _, m := range t.TermMonths {
		if m < 1 || m > 120 {
			v.Add("terms.termMonths", "each term 1-120 months")
		}
	}
}

// Eligibility limits which sale prices a program accepts (minor units, "" = no limit).
type Eligibility struct {
	MinPriceMinor string `json:"minPriceMinor"`
	MaxPriceMinor string `json:"maxPriceMinor"`
}
