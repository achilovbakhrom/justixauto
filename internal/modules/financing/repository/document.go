package repository

import (
	"context"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/database"
)

func (r *Store) CreateDocumentRequest(ctx context.Context, d *model.DocumentRequest) error {
	return r.create(ctx, d)
}

func (r *Store) DocumentRequest(ctx context.Context, id string) (*model.DocumentRequest, error) {
	var d model.DocumentRequest
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&d).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &d, nil
}

func (r *Store) DocumentRequests(ctx context.Context, applicationID string) ([]model.DocumentRequest, error) {
	ds := []model.DocumentRequest{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("created_at, id").Find(&ds).Error
	return ds, database.Translate(err)
}

func (r *Store) UpdateDocumentRequest(ctx context.Context, d *model.DocumentRequest, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.DocumentRequest{}, d.ID, expected,
		map[string]any{"status": d.Status, "status_note": d.StatusNote, "updated_at": d.UpdatedAt})
	if err == nil {
		d.Version = expected + 1
	}
	return err
}

func (r *Store) CreateSubmission(ctx context.Context, s *model.Submission) error {
	return r.create(ctx, s)
}

func (r *Store) NextSubmissionNumber(ctx context.Context, requestID string) (int, error) {
	return r.nextNumber(ctx, &model.Submission{}, "request_id", requestID)
}

func (r *Store) Submissions(ctx context.Context, requestID string) ([]model.Submission, error) {
	ss := []model.Submission{}
	err := r.db.WithContext(ctx).Where("request_id = ?", requestID).Order("number").Find(&ss).Error
	return ss, database.Translate(err)
}
