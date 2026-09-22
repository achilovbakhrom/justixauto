package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"justixauto/internal/platform/apperr"
)

// memoryRepo is an in-memory CompanyRepository with the same version and
// uniqueness semantics as the PostgreSQL one.
type memoryRepo struct{ companies map[string]Company }

func newMemoryRepo() *memoryRepo { return &memoryRepo{companies: map[string]Company{}} }

func (r *memoryRepo) duplicate(c *Company) bool {
	for _, other := range r.companies {
		if other.ID != c.ID && strings.EqualFold(other.Country, c.Country) && other.RegistrationNumber == c.RegistrationNumber {
			return true
		}
	}
	return false
}

func (r *memoryRepo) Create(_ context.Context, c *Company) error {
	if r.duplicate(c) {
		return fmt.Errorf("%w: duplicate", apperr.ErrConflict)
	}
	r.companies[c.ID] = *c
	return nil
}

func (r *memoryRepo) Get(_ context.Context, id string) (*Company, error) {
	c, ok := r.companies[id]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &c, nil
}

func (r *memoryRepo) List(_ context.Context, f CompanyFilter) ([]Company, error) {
	out := []Company{}
	for _, c := range r.companies {
		if (f.Kind == "" || c.Kind == f.Kind) && (f.Status == "" || c.Status == f.Status) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *memoryRepo) Update(_ context.Context, c *Company, expected int64) error {
	stored, ok := r.companies[c.ID]
	if !ok {
		return apperr.ErrNotFound
	}
	if stored.Version != expected {
		return apperr.ErrStale
	}
	if r.duplicate(c) {
		return fmt.Errorf("%w: duplicate", apperr.ErrConflict)
	}
	c.Version = expected + 1
	r.companies[c.ID] = *c
	return nil
}

var fixedNow = time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)

func newTestService() *CompanyService {
	return NewCompanyService(newMemoryRepo(), func() time.Time { return fixedNow })
}

func validCreate() CreateCompanyInput {
	return CreateCompanyInput{Kind: KindBank, Name: " Test Bank ", Country: "Uzbekistan", Region: "Tashkent", RegistrationNumber: "123456789"}
}

func fieldErrors(t *testing.T, err error) map[string]string {
	t.Helper()
	var v *apperr.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("want validation error, got %v", err)
	}
	return v.Fields
}

func TestCreateStartsAsDraftAndTrimsInput(t *testing.T) {
	c, err := newTestService().Create(context.Background(), validCreate())
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusDraft || c.Version != 1 || c.Name != "Test Bank" || !c.CreatedAt.Equal(fixedNow) {
		t.Fatalf("unexpected company: %+v", c)
	}
}

func TestCreateValidation(t *testing.T) {
	cases := map[string]struct {
		edit  func(*CreateCompanyInput)
		field string
	}{
		"unknown kind":           {func(in *CreateCompanyInput) { in.Kind = "dealer" }, "kind"},
		"blank name":             {func(in *CreateCompanyInput) { in.Name = "   " }, "name"},
		"long name":              {func(in *CreateCompanyInput) { in.Name = strings.Repeat("я", 201) }, "name"},
		"missing registration":   {func(in *CreateCompanyInput) { in.RegistrationNumber = "" }, "registrationNumber"},
		"region without country": {func(in *CreateCompanyInput) { in.Country = "" }, "region"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := validCreate()
			tc.edit(&in)
			_, err := newTestService().Create(context.Background(), in)
			if _, ok := fieldErrors(t, err)[tc.field]; !ok {
				t.Fatalf("want error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestCreateRejectsDuplicateCountryAndRegistration(t *testing.T) {
	s := newTestService()
	if _, err := s.Create(context.Background(), validCreate()); err != nil {
		t.Fatal(err)
	}
	dup := validCreate()
	dup.Country, dup.Name = "UZBEKISTAN", "Other name"
	if _, err := s.Create(context.Background(), dup); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestUpdateKeepsIDAndBumpsVersion(t *testing.T) {
	s := newTestService()
	c, _ := s.Create(context.Background(), validCreate())
	updated, err := s.Update(context.Background(), c.ID, 1, UpdateCompanyInput{Name: "Renamed", Country: "Uzbekistan", RegistrationNumber: "123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != c.ID || updated.Version != 2 || updated.Name != "Renamed" || updated.Kind != KindBank {
		t.Fatalf("unexpected update: %+v", updated)
	}
}

func TestUpdateWithStaleVersionFails(t *testing.T) {
	s := newTestService()
	c, _ := s.Create(context.Background(), validCreate())
	in := UpdateCompanyInput{Name: "A", Country: "Uzbekistan", RegistrationNumber: "123456789"}
	if _, err := s.Update(context.Background(), c.ID, 1, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(context.Background(), c.ID, 1, in); !errors.Is(err, apperr.ErrStale) {
		t.Fatalf("want stale, got %v", err)
	}
}

func TestUnknownOrMalformedIDIsNotFound(t *testing.T) {
	s := newTestService()
	for _, id := range []string{"not-a-uuid", "7f1c6c56-6a4f-4d8e-9d59-1f9a0c1d2e3f"} {
		if _, err := s.Get(context.Background(), id); !errors.Is(err, apperr.ErrNotFound) {
			t.Fatalf("%s: want not found, got %v", id, err)
		}
	}
}

func TestStatusTransitions(t *testing.T) {
	s := newTestService()
	ctx := context.Background()
	c, _ := s.Create(ctx, validCreate())

	if _, err := s.ChangeStatus(ctx, c.ID, 1, ChangeStatusInput{Status: StatusSuspended, Reason: "x"}); fieldErrors(t, err)["status"] == "" {
		t.Fatal("draft → suspended must be rejected")
	}
	if _, err := s.ChangeStatus(ctx, c.ID, 1, ChangeStatusInput{Status: StatusActive, Reason: "  "}); fieldErrors(t, err)["reason"] == "" {
		t.Fatal("activation without reason must be rejected")
	}
	steps := []CompanyStatus{StatusActive, StatusSuspended, StatusActive}
	for i, next := range steps {
		got, err := s.ChangeStatus(ctx, c.ID, int64(i+1), ChangeStatusInput{Status: next, Reason: "approved by admin"})
		if err != nil {
			t.Fatalf("step %d → %s: %v", i, next, err)
		}
		if got.Status != next || got.StatusReason != "approved by admin" {
			t.Fatalf("step %d: %+v", i, got)
		}
	}
}

func TestListValidatesFiltersAndCapsLimit(t *testing.T) {
	s := newTestService()
	if _, err := s.List(context.Background(), CompanyFilter{Status: "deleted"}); fieldErrors(t, err)["status"] == "" {
		t.Fatal("unknown status filter must be rejected")
	}
	if _, err := s.List(context.Background(), CompanyFilter{Limit: 10_000}); err != nil {
		t.Fatal(err)
	}
}
