package model

import "time"

type Application struct {
	ID                  string  `gorm:"primaryKey;type:uuid"`
	SellerCompanyID     string  `gorm:"type:uuid"`
	ProviderCompanyID   string  `gorm:"type:uuid"`
	DealID              string  `gorm:"type:uuid"`
	ProgramID           *string `gorm:"type:uuid"`
	ProgramVersion      *int
	Calculation         []byte `gorm:"type:jsonb"`
	CalculationDigest   string
	Status              string
	Snapshot            []byte `gorm:"type:jsonb"`
	CurrentTermsVersion *int
	Version             int64
	CreatedBy           string `gorm:"type:uuid"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	SubmittedAt         *time.Time
}

func (Application) TableName() string { return "financing.applications" }

type TermsVersion struct {
	ApplicationID string `gorm:"primaryKey;type:uuid"`
	Number        int    `gorm:"primaryKey"`
	Calculation   []byte `gorm:"type:jsonb"`
	Note          string
	CreatedBy     string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (TermsVersion) TableName() string { return "financing.terms_versions" }

type Message struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	Seq           int64  `gorm:"->"`
	ApplicationID string `gorm:"type:uuid"`
	Kind          string
	RequestID     *string `gorm:"type:uuid"`
	Note          string
	TermsVersion  *int
	CompanyID     string `gorm:"type:uuid"`
	ActorUserID   string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (Message) TableName() string { return "financing.messages" }
