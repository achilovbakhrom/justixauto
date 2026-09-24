// Package model holds identity's GORM models, enums and the permission
// catalog. It imports internal/pkg only; it must never import service,
// repository or handler.
package model

import "time"

// CompanyKind: a seller using the Realization app, or a provider (bank, MFO,
// insurance company) connected through Admin → Integrations.
type CompanyKind string

const (
	KindSeller    CompanyKind = "seller"
	KindBank      CompanyKind = "bank"
	KindMFO       CompanyKind = "mfo"
	KindInsurance CompanyKind = "insurance"
)

func (k CompanyKind) Valid() bool {
	switch k {
	case KindSeller, KindBank, KindMFO, KindInsurance:
		return true
	}
	return false
}

func (k CompanyKind) Provider() bool { return k == KindBank || k == KindMFO || k == KindInsurance }

// CompanyAccess is platform access, not legal/compliance verification.
type CompanyAccess string

const (
	AccessDraft     CompanyAccess = "draft"
	AccessActive    CompanyAccess = "active"
	AccessSuspended CompanyAccess = "suspended"
)

// Company is a registered organization. Requisites can change; the ID never does.
type Company struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	Kind               CompanyKind
	Name               string
	LegalName          string
	Country            string // label as entered
	CountryKey         string // catalogue key when chosen from the list
	Region             string
	RegionKey          string
	RegistrationNumber string
	Email              string
	Address            string
	Phone              string
	Status             CompanyAccess
	StatusReason       string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Company) TableName() string { return "identity.companies" }

// CompanyFilter narrows List; zero values mean "any".
type CompanyFilter struct {
	Query  string // part of the name
	Kind   CompanyKind
	Access CompanyAccess
	Limit  int
	Offset int
}
