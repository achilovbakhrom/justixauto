// Package identity owns companies (organizations), branches, users, roles and
// memberships. Other modules reference them by ID only.
package identity

import "time"

// CompanyKind is the type of participant: a seller using the Realization app,
// or a bank / MFO / insurer connected through Admin → Integrations.
type CompanyKind string

const (
	KindSeller  CompanyKind = "seller"
	KindBank    CompanyKind = "bank"
	KindMFO     CompanyKind = "mfo"
	KindInsurer CompanyKind = "insurer"
)

func (k CompanyKind) Valid() bool {
	switch k {
	case KindSeller, KindBank, KindMFO, KindInsurer:
		return true
	}
	return false
}

// CompanyStatus is platform access, not legal/compliance verification.
type CompanyStatus string

const (
	StatusDraft     CompanyStatus = "draft"
	StatusActive    CompanyStatus = "active"
	StatusSuspended CompanyStatus = "suspended"
)

// Company is a registered organization. Requisites can change; the ID never does.
type Company struct {
	ID                 string        `gorm:"primaryKey;type:uuid" json:"id"`
	Kind               CompanyKind   `json:"kind"`
	Name               string        `json:"name"`
	Country            string        `json:"country"`
	Region             string        `json:"region"`
	RegistrationNumber string        `json:"registrationNumber"`
	Status             CompanyStatus `json:"status"`
	StatusReason       string        `json:"statusReason"`
	Version            int64         `json:"version"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

func (Company) TableName() string { return "identity.companies" }
