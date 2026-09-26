package service

import (
	"testing"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
)

// User decision 2026-09-26: country, registration number and email are
// optional company requisites; only the name is required here.
func TestCompanyInputApplyOptionalRequisites(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors"}
	in.apply(&v, c)
	if err := v.Err(); err != nil {
		t.Fatalf("minimal company input should validate: %v", err)
	}
	if c.Country != "" || c.RegistrationNumber != "" || c.Email != "" {
		t.Fatalf("optional fields should stay empty: %+v", c)
	}
}

func TestCompanyInputApplyNameStillRequired(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{}
	in.apply(&v, c)
	if err := v.Err(); err == nil {
		t.Fatal("empty company name should still fail validation")
	}
}

func TestCompanyInputApplyRegionRequiresCountry(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors", Region: &Label{Label: "Tashkent"}}
	in.apply(&v, c)
	if err := v.Err(); err == nil {
		t.Fatal("a region without a country should fail validation")
	}
}

func TestCompanyInputApplyInvalidEmailWhenGiven(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors", Email: "not-an-email"}
	in.apply(&v, c)
	if err := v.Err(); err == nil {
		t.Fatal("a malformed email should still fail validation when given")
	}
}

func TestOptionalEmail(t *testing.T) {
	var v apperr.Validation
	if got := optionalEmail(&v, "field", ""); got != "" {
		t.Fatalf("empty email should stay empty, got %q", got)
	}
	if err := v.Err(); err != nil {
		t.Fatalf("empty email should not add a validation error: %v", err)
	}
	if got := optionalEmail(&v, "field", "person@example.test"); got != "person@example.test" {
		t.Fatalf("valid email should pass through, got %q", got)
	}
	optionalEmail(&v, "field", "not-an-email")
	if err := v.Err(); err == nil {
		t.Fatal("a malformed non-empty email should fail validation")
	}
}
