package model

import "time"

const (
	PermRead               = "commerce.read"
	PermPartnershipsManage = "commerce.partnerships.manage"
)

type PartnershipStatus string

const (
	Requested PartnershipStatus = "requested"
	Active    PartnershipStatus = "active"
	Declined  PartnershipStatus = "declined"
	Withdrawn PartnershipStatus = "withdrawn"
	Ended     PartnershipStatus = "ended"
)

type Partnership struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	RequesterCompanyID string `gorm:"type:uuid"`
	RecipientCompanyID string `gorm:"type:uuid"`
	Status             PartnershipStatus
	StatusReason       string
	RequestedBy        string `gorm:"type:uuid"`
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ActivatedAt        *time.Time
	ClosedAt           *time.Time
}

func (Partnership) TableName() string { return "commerce.partnerships" }

// Counterparty returns the other company from companyID's point of view.
func (p *Partnership) Counterparty(companyID string) string {
	if p.RequesterCompanyID == companyID {
		return p.RecipientCompanyID
	}
	return p.RequesterCompanyID
}

// AllowedActions lists what companyID may do with the partnership now.
func (p *Partnership) AllowedActions(companyID string) []string {
	switch {
	case p.Status == Requested && p.RecipientCompanyID == companyID:
		return []string{"accept", "decline"}
	case p.Status == Requested:
		return []string{"withdraw"}
	case p.Status == Active:
		return []string{"end"}
	}
	return []string{}
}

type Event struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Seq          int64  `gorm:"->"`
	CompanyID    string `gorm:"type:uuid"`
	EventType    string
	ResourceType string
	ResourceID   string `gorm:"type:uuid"`
	ActorUserID  string `gorm:"type:uuid"`
	OccurredAt   time.Time
	Reason       string
	Details      []byte `gorm:"type:jsonb"`
}

func (Event) TableName() string { return "commerce.events" }

// PartnershipFilter narrows a partnership listing.
type PartnershipFilter struct {
	CompanyID string
	Status    PartnershipStatus
	Limit     int
	Offset    int
}

// Transition describes who may move a partnership from one state to another.
type Transition struct {
	From, To  PartnershipStatus
	Recipient bool // only the recipient (else only the requester, unless Any)
	Any       bool // either company
	Reason    bool // reason required
}

// Transitions are the partnership decisions: accept, decline, withdraw, end.
var Transitions = map[string]Transition{
	"accept":   {From: Requested, To: Active, Recipient: true},
	"decline":  {From: Requested, To: Declined, Recipient: true, Reason: true},
	"withdraw": {From: Requested, To: Withdrawn, Reason: true},
	"end":      {From: Active, To: Ended, Any: true, Reason: true},
}
