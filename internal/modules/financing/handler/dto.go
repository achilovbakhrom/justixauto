package handler

import (
	"encoding/json"
	"strconv"
	"time"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/modules/financing/service"
	"justixauto/internal/pkg/httpx"
)

type programVersionDTO struct {
	Number                   string             `json:"number"`
	Name                     string             `json:"name"`
	Currency                 string             `json:"currency"`
	Terms                    model.ProgramTerms `json:"terms"`
	Eligibility              model.Eligibility  `json:"eligibility"`
	CalculationPolicyID      string             `json:"calculationPolicyId"`
	CalculationPolicyVersion int                `json:"calculationPolicyVersion"`
	CreatedAt                time.Time          `json:"createdAt"`
}

type programDTO struct {
	ID               string              `json:"id"`
	Provider         map[string]string   `json:"provider"`
	Status           string              `json:"status"`
	StatusReason     string              `json:"statusReason"`
	PublishedVersion *int                `json:"publishedVersion"`
	Versions         []programVersionDTO `json:"versions"`
	Revision         string              `json:"revision"`
}

func toProgram(v *service.ProgramView) programDTO {
	d := programDTO{
		ID: v.Program.ID, Provider: map[string]string{"id": v.Provider.ID, "name": v.Provider.Name, "kind": v.Provider.Kind},
		Status: v.Program.Status, StatusReason: v.Program.StatusReason, PublishedVersion: v.Program.PublishedVersion,
		Versions: []programVersionDTO{}, Revision: httpx.Revision(v.Program.Version),
	}
	for i := range v.Versions {
		pv := &v.Versions[i]
		t, e := pv.Decode()
		d.Versions = append(d.Versions, programVersionDTO{
			Number: strconv.Itoa(pv.Number), Name: pv.Name, Currency: pv.Currency,
			Terms: t, Eligibility: e, CalculationPolicyID: pv.PolicyID, CalculationPolicyVersion: pv.PolicyVersion, CreatedAt: pv.CreatedAt,
		})
	}
	return d
}

type termsDTO struct {
	Number      int             `json:"number"`
	Calculation json.RawMessage `json:"calculation"`
	Note        string          `json:"note"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type messageDTO struct {
	Kind         string    `json:"kind"`
	RequestID    *string   `json:"requestId"`
	TermsVersion *int      `json:"termsVersion"`
	Note         string    `json:"note"`
	Side         string    `json:"side"`
	ActorID      string    `json:"actorId"`
	OccurredAt   time.Time `json:"occurredAt"`
}

type applicationDTO struct {
	ID                  string          `json:"id"`
	Side                string          `json:"side"`
	SellerCompanyID     string          `json:"sellerCompanyId"`
	ProviderCompanyID   string          `json:"providerCompanyId"`
	RetailDealID        string          `json:"retailDealId"`
	ProgramID           *string         `json:"programId"`
	ProgramVersion      *int            `json:"programVersion"`
	Calculation         json.RawMessage `json:"calculation"`
	CalculationDigest   string          `json:"calculationDigest"`
	Status              string          `json:"status"`
	Snapshot            json.RawMessage `json:"snapshot"`
	CurrentTermsVersion *int            `json:"currentTermsVersion"`
	Terms               []termsDTO      `json:"terms,omitempty"`
	History             []messageDTO    `json:"history,omitempty"`
	AllowedActions      []string        `json:"allowedActions"`
	Revision            string          `json:"revision"`
}

func raw(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

func toApplication(companyID string, v *service.ApplicationView) applicationDTO {
	a := v.Application
	seller := a.SellerCompanyID == companyID
	side := map[bool]string{true: "seller", false: "provider"}[seller]
	d := applicationDTO{
		ID: a.ID, Side: side, SellerCompanyID: a.SellerCompanyID, ProviderCompanyID: a.ProviderCompanyID,
		RetailDealID: a.DealID, ProgramID: a.ProgramID, ProgramVersion: a.ProgramVersion, Calculation: raw(a.Calculation),
		CalculationDigest: a.CalculationDigest, Status: a.Status, Snapshot: raw(a.Snapshot), CurrentTermsVersion: a.CurrentTermsVersion,
		AllowedActions: []string{}, Revision: httpx.Revision(a.Version),
	}
	switch {
	case seller && a.Status == "draft":
		d.AllowedActions = []string{"edit", "submit"}
	case seller && a.Status == "needs-info":
		d.AllowedActions = []string{"respond"}
	case seller && a.Status == "terms":
		d.AllowedActions = []string{"agree", "counter"}
	case !seller && a.Status == "submitted":
		d.AllowedActions = []string{"take"}
	case !seller && a.Status == "review":
		d.AllowedActions = []string{"request", "terms", "decline"}
	}
	if v.Terms != nil {
		d.Terms = []termsDTO{}
		for _, t := range v.Terms {
			d.Terms = append(d.Terms, termsDTO{Number: t.Number, Calculation: t.Calculation, Note: t.Note, CreatedAt: t.CreatedAt})
		}
		d.History = []messageDTO{}
		for _, m := range v.History {
			d.History = append(d.History, messageDTO{
				Kind: m.Kind, RequestID: m.RequestID, TermsVersion: m.TermsVersion, Note: m.Note,
				Side: map[bool]string{true: "seller", false: "provider"}[m.CompanyID == a.SellerCompanyID], ActorID: m.ActorUserID, OccurredAt: m.CreatedAt,
			})
		}
	}
	return d
}

// publishProgramRequest publishes a financing program version.
type publishProgramRequest struct {
	ProgramVersion int `json:"programVersion"`
}

// withdrawProgramRequest withdraws a published financing program.
type withdrawProgramRequest struct {
	Reason string `json:"reason"`
}

// submitApplicationRequest submits a draft financing application for review.
type submitApplicationRequest struct {
	Confirmation      bool   `json:"confirmation"`
	DealRevision      string `json:"dealRevision"`
	CalculationDigest string `json:"calculationDigest"`
}

// requestDocumentRequest requests a supporting document on a financing application.
type requestDocumentRequest struct {
	Title        string `json:"title"`
	Requirements string `json:"requirements"`
}

// submitDocumentRequest submits a document for a requested attachment.
type submitDocumentRequest struct {
	AttachmentBindingID string `json:"attachmentBindingId"`
	Note                string `json:"note"`
}

// decideDocumentRequest accepts, returns or cancels a document submission.
type decideDocumentRequest struct {
	SubmissionVersion int    `json:"submissionVersion"`
	Confirmation      bool   `json:"confirmation"`
	Note              string `json:"note"`
	Reason            string `json:"reason"`
}

type submissionDTO struct {
	Version   int       `json:"version"`
	FileID    string    `json:"fileId"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type documentRequestDTO struct {
	ID            string          `json:"id"`
	ApplicationID string          `json:"applicationId"`
	Title         string          `json:"title"`
	Requirements  string          `json:"requirements"`
	Status        string          `json:"status"`
	StatusNote    string          `json:"statusNote"`
	Submissions   []submissionDTO `json:"submissions"`
	Revision      string          `json:"revision"`
}

func toDocumentRequest(v *service.DocumentRequestView) documentRequestDTO {
	d := documentRequestDTO{
		ID: v.Request.ID, ApplicationID: v.Request.ApplicationID, Title: v.Request.Title,
		Requirements: v.Request.Requirements, Status: v.Request.Status, StatusNote: v.Request.StatusNote,
		Submissions: []submissionDTO{}, Revision: httpx.Revision(v.Request.Version),
	}
	for _, s := range v.Submissions {
		d.Submissions = append(d.Submissions, submissionDTO{Version: s.Number, FileID: s.FileID, Note: s.Note, CreatedAt: s.CreatedAt})
	}
	return d
}
