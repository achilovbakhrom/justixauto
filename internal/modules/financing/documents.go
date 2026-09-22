package financing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/validate"
)

// Files shares an uploaded file with another company (implemented by the
// documents module). It fails with apperr.ErrNotFound if the file is not the
// owner's.
type Files interface {
	Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error
}

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

func (r *repo) documentRequest(ctx context.Context, id string) (*DocumentRequest, error) {
	var d DocumentRequest
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&d).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &d, nil
}

func (r *repo) documentRequests(ctx context.Context, applicationID string) ([]DocumentRequest, error) {
	ds := []DocumentRequest{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("created_at, id").Find(&ds).Error
	return ds, database.Translate(err)
}

func (r *repo) updateDocumentRequest(ctx context.Context, d *DocumentRequest, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &DocumentRequest{}, d.ID, expected,
		map[string]any{"status": d.Status, "status_note": d.StatusNote, "updated_at": d.UpdatedAt})
	if err == nil {
		d.Version = expected + 1
	}
	return err
}

func (r *repo) submissions(ctx context.Context, requestID string) ([]Submission, error) {
	ss := []Submission{}
	err := r.db.WithContext(ctx).Where("request_id = ?", requestID).Order("number").Find(&ss).Error
	return ss, database.Translate(err)
}

// RequestDocument: the provider asks for a document on an agreed application.
func (s *Service) RequestDocument(ctx context.Context, p *auth.Principal, applicationID, title, requirements string) (*DocumentRequest, error) {
	var v apperr.Validation
	title = validate.Text(&v, "title", title, 1, 200)
	requirements = validate.Text(&v, "requirements", requirements, 0, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := validate.IDs(applicationID); err != nil {
		return nil, err
	}
	a, err := s.r.application(ctx, p.CompanyID, applicationID)
	if err != nil {
		return nil, err
	}
	if a.ProviderCompanyID != p.CompanyID {
		return nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the provider requests documents")
	}
	if a.Status != "agreed" {
		return nil, apperr.New(apperr.ErrConflict, "invalid_transition", "documents are requested after agreement")
	}
	now := s.clock()
	d := &DocumentRequest{ID: uuid.NewString(), ApplicationID: a.ID, Title: title, Requirements: requirements, Status: "requested",
		Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
	return d, s.r.create(ctx, d)
}

// documentRequest loads a request with its application, checking the side.
func (s *Service) documentRequest(ctx context.Context, r *repo, p *auth.Principal, id string, expected int64, side string) (*DocumentRequest, *Application, error) {
	if err := validate.IDs(id); err != nil {
		return nil, nil, err
	}
	d, err := r.documentRequest(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	a, err := r.application(ctx, p.CompanyID, d.ApplicationID)
	if err != nil {
		return nil, nil, err
	}
	if expected >= 0 && d.Version != expected {
		return nil, nil, apperr.ErrStale
	}
	if side != "" && (side == "seller") != (a.SellerCompanyID == p.CompanyID) {
		return nil, nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the "+side+" can do this")
	}
	return d, a, nil
}

// SubmitDocument: the seller submits an uploaded file as the next version;
// the file is shared with the provider for this request.
func (s *Service) SubmitDocument(ctx context.Context, p *auth.Principal, id string, expected int64, fileID, note string) (*DocumentRequest, error) {
	var v apperr.Validation
	note = validate.Text(&v, "note", note, 0, 2000)
	if validate.IDs(fileID) != nil {
		v.Add("attachmentBindingId", "an uploaded file ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var d *DocumentRequest
	err := s.r.tx(ctx, func(r *repo) error {
		var a *Application
		var err error
		if d, a, err = s.documentRequest(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if d.Status != "requested" && d.Status != "changes" {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the request is "+d.Status)
		}
		if err := s.files.Share(database.WithTx(ctx, r.db), p.CompanyID, fileID, a.ProviderCompanyID, "financing.document-request", d.ID); err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
				return apperr.FieldError("attachmentBindingId", "not a file of your company")
			}
			return err
		}
		n, err := r.nextNumber(ctx, &Submission{}, "request_id", d.ID)
		if err != nil {
			return err
		}
		if err := r.create(ctx, &Submission{RequestID: d.ID, Number: n, FileID: fileID, Note: note, CreatedBy: p.UserID, CreatedAt: s.clock()}); err != nil {
			return err
		}
		d.Status, d.StatusNote, d.UpdatedAt = "review", "", s.clock()
		return r.updateDocumentRequest(ctx, d, expected)
	})
	return d, err
}

// DecideDocument: the provider accepts (confirmation) or returns (note) the
// exact latest submission, or cancels an open request (reason).
func (s *Service) DecideDocument(ctx context.Context, p *auth.Principal, id string, expected int64, action string, submission int, confirmation bool, note string) (*DocumentRequest, error) {
	var v apperr.Validation
	switch action {
	case "accept":
		if !confirmation {
			v.Add("confirmation", "confirm the document meets the requirements")
		}
	case "return":
		note = validate.Text(&v, "note", note, 1, 2000)
	case "cancel":
		note = validate.Reason(&v, note)
	default:
		return nil, apperr.ErrNotFound
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var d *DocumentRequest
	err := s.r.tx(ctx, func(r *repo) error {
		var err error
		if d, _, err = s.documentRequest(ctx, r, p, id, expected, "provider"); err != nil {
			return err
		}
		switch {
		case action == "cancel" && (d.Status == "requested" || d.Status == "changes"):
			d.Status = "cancelled"
		case action != "cancel" && d.Status == "review":
			ss, err := r.submissions(ctx, d.ID)
			if err != nil {
				return err
			}
			if len(ss) == 0 || ss[len(ss)-1].Number != submission {
				return apperr.New(apperr.ErrConflict, "submission_changed", "decide on the latest submission; reload")
			}
			d.Status = map[string]string{"accept": "accepted", "return": "changes"}[action]
		default:
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" a request that is "+d.Status)
		}
		d.StatusNote, d.UpdatedAt = note, s.clock()
		return r.updateDocumentRequest(ctx, d, expected)
	})
	return d, err
}

type DocumentRequestView struct {
	Request     DocumentRequest
	Submissions []Submission
}

func (s *Service) DocumentRequests(ctx context.Context, p *auth.Principal, applicationID string) ([]DocumentRequestView, error) {
	if err := validate.IDs(applicationID); err != nil {
		return nil, err
	}
	if _, err := s.r.application(ctx, p.CompanyID, applicationID); err != nil {
		return nil, err
	}
	ds, err := s.r.documentRequests(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	out := make([]DocumentRequestView, 0, len(ds))
	for _, d := range ds {
		ss, err := s.r.submissions(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, DocumentRequestView{Request: d, Submissions: ss})
	}
	return out, nil
}

func (s *Service) DocumentRequestView(ctx context.Context, p *auth.Principal, id string) (*DocumentRequestView, error) {
	d, _, err := s.documentRequest(ctx, s.r, p, id, -1, "")
	if err != nil {
		return nil, err
	}
	ss, err := s.r.submissions(ctx, d.ID)
	return &DocumentRequestView{Request: *d, Submissions: ss}, err
}
