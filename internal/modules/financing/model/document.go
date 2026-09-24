package model

import "time"

// DocumentRequest: after agreement the provider asks for documents; the seller
// submits numbered versions; the provider accepts or returns them.
//
//	requested/changes → review    (seller submits a new version)
//	review → accepted             (provider, with confirmation)
//	review → changes              (provider, with a note)
//	requested/changes → cancelled (provider, with a reason)
//
// An accepted document is not a signature, contract or funding.
type DocumentRequest struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	ApplicationID string `gorm:"type:uuid"`
	Title         string
	Requirements  string
	Status        string
	StatusNote    string
	Version       int64
	CreatedBy     string `gorm:"type:uuid"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (DocumentRequest) TableName() string { return "financing.document_requests" }

type Submission struct {
	RequestID string `gorm:"primaryKey;type:uuid"`
	Number    int    `gorm:"primaryKey"`
	FileID    string `gorm:"type:uuid"`
	Note      string
	CreatedBy string `gorm:"type:uuid"`
	CreatedAt time.Time
}

func (Submission) TableName() string { return "financing.document_submissions" }
