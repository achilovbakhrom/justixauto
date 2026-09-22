package identity

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
)

type CreateCompanyInput struct {
	Kind               CompanyKind `json:"kind"`
	Name               string      `json:"name"`
	Country            string      `json:"country"`
	Region             string      `json:"region"`
	RegistrationNumber string      `json:"registrationNumber"`
}

// UpdateCompanyInput edits requisites. Kind and ID are immutable.
type UpdateCompanyInput struct {
	Name               string `json:"name"`
	Country            string `json:"country"`
	Region             string `json:"region"`
	RegistrationNumber string `json:"registrationNumber"`
}

type ChangeStatusInput struct {
	Status CompanyStatus `json:"status"`
	Reason string        `json:"reason"`
}

type CompanyService struct {
	repo CompanyRepository
	now  func() time.Time
}

func NewCompanyService(repo CompanyRepository, now func() time.Time) *CompanyService {
	return &CompanyService{repo: repo, now: now}
}

// Create registers a company in draft status; it must be activated separately.
func (s *CompanyService) Create(ctx context.Context, in CreateCompanyInput) (*Company, error) {
	var v apperr.Validation
	if !in.Kind.Valid() {
		v.Add("kind", "must be one of seller, bank, mfo, insurer")
	}
	req := normalizeRequisites(in.Name, in.Country, in.Region, in.RegistrationNumber, &v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	c := &Company{
		ID:                 uuid.NewString(),
		Kind:               in.Kind,
		Name:               req.name,
		Country:            req.country,
		Region:             req.region,
		RegistrationNumber: req.registration,
		Status:             StatusDraft,
		Version:            1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CompanyService) Get(ctx context.Context, id string) (*Company, error) {
	if uuid.Validate(id) != nil {
		return nil, apperr.ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

func (s *CompanyService) List(ctx context.Context, f CompanyFilter) ([]Company, error) {
	var v apperr.Validation
	if f.Kind != "" && !f.Kind.Valid() {
		v.Add("kind", "unknown kind")
	}
	if f.Status != "" && !validStatus(f.Status) {
		v.Add("status", "unknown status")
	}
	if f.Limit < 0 || f.Offset < 0 {
		v.Add("limit", "limit and offset must not be negative")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if f.Limit == 0 || f.Limit > 200 {
		f.Limit = 50
	}
	return s.repo.List(ctx, f)
}

// Update changes requisites. The ID, memberships and history stay the same.
func (s *CompanyService) Update(ctx context.Context, id string, expectedVersion int64, in UpdateCompanyInput) (*Company, error) {
	c, err := s.load(ctx, id, expectedVersion)
	if err != nil {
		return nil, err
	}
	var v apperr.Validation
	req := normalizeRequisites(in.Name, in.Country, in.Region, in.RegistrationNumber, &v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	c.Name, c.Country, c.Region, c.RegistrationNumber = req.name, req.country, req.region, req.registration
	c.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, c, expectedVersion); err != nil {
		return nil, err
	}
	return c, nil
}

// allowedTransitions: activation needs a reason; suspension keeps history and
// can be reversed. Access status is not legal or compliance verification.
var allowedTransitions = map[CompanyStatus][]CompanyStatus{
	StatusDraft:     {StatusActive},
	StatusActive:    {StatusSuspended},
	StatusSuspended: {StatusActive},
}

func (s *CompanyService) ChangeStatus(ctx context.Context, id string, expectedVersion int64, in ChangeStatusInput) (*Company, error) {
	c, err := s.load(ctx, id, expectedVersion)
	if err != nil {
		return nil, err
	}
	var v apperr.Validation
	reason := strings.TrimSpace(in.Reason)
	if !canTransition(c.Status, in.Status) {
		v.Add("status", "cannot change from "+string(c.Status)+" to "+string(in.Status))
	}
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		v.Add("reason", "required, at most 500 characters")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	c.Status, c.StatusReason, c.UpdatedAt = in.Status, reason, s.now().UTC()
	if err := s.repo.Update(ctx, c, expectedVersion); err != nil {
		return nil, err
	}
	return c, nil
}

// load fetches the company and rejects early if the client's version is stale,
// so rules are never evaluated against a state the client has not seen.
func (s *CompanyService) load(ctx context.Context, id string, expectedVersion int64) (*Company, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Version != expectedVersion {
		return nil, apperr.ErrStale
	}
	return c, nil
}

func canTransition(from, to CompanyStatus) bool {
	for _, allowed := range allowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func validStatus(s CompanyStatus) bool {
	_, ok := allowedTransitions[s]
	return ok
}

type requisites struct{ name, country, region, registration string }

func normalizeRequisites(name, country, region, registration string, v *apperr.Validation) requisites {
	r := requisites{
		name:         strings.TrimSpace(name),
		country:      strings.TrimSpace(country),
		region:       strings.TrimSpace(region),
		registration: strings.TrimSpace(registration),
	}
	checkLength(v, "name", r.name, 200)
	checkLength(v, "country", r.country, 100)
	checkLength(v, "registrationNumber", r.registration, 64)
	if utf8.RuneCountInString(r.region) > 100 {
		v.Add("region", "at most 100 characters")
	}
	if r.region != "" && r.country == "" {
		v.Add("region", "requires a country")
	}
	return r
}

func checkLength(v *apperr.Validation, field, value string, max int) {
	if n := utf8.RuneCountInString(value); n == 0 || n > max {
		v.Add(field, "required, at most "+strconv.Itoa(max)+" characters")
	}
}
