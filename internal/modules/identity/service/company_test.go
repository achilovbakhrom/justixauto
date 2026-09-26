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
	in.apply(&v, c, false)
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
	in.apply(&v, c, false)
	if err := v.Err(); err == nil {
		t.Fatal("empty company name should still fail validation")
	}
}

func TestCompanyInputApplyRegionRequiresCountry(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors", Region: &Label{Label: "Tashkent"}}
	in.apply(&v, c, false)
	if err := v.Err(); err == nil {
		t.Fatal("a region without a country should fail validation")
	}
}

func TestCompanyInputApplyInvalidEmailWhenGiven(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors", Email: "not-an-email"}
	in.apply(&v, c, false)
	if err := v.Err(); err == nil {
		t.Fatal("a malformed email should still fail validation when given")
	}
}

// User decision (business-logic.md §9): the registration number arrives from
// a government-source integration; edits must never erase an existing value.
func TestCompanyInputApplyKeepsRegistrationWhenOmittedOnUpdate(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{RegistrationNumber: "REG-1"}
	in := CompanyInput{Name: "Bare Motors"}
	in.apply(&v, c, true)
	if err := v.Err(); err != nil {
		t.Fatalf("update without registration should validate: %v", err)
	}
	if c.RegistrationNumber != "REG-1" {
		t.Fatalf("omitted registration should keep the stored value, got %q", c.RegistrationNumber)
	}
}

func TestCompanyInputApplyKeepsRegistrationWhenEmptyOnUpdate(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{RegistrationNumber: "REG-1"}
	in := CompanyInput{Name: "Bare Motors", Registration: ""}
	in.apply(&v, c, true)
	if err := v.Err(); err != nil {
		t.Fatalf("update with empty registration should validate: %v", err)
	}
	if c.RegistrationNumber != "REG-1" {
		t.Fatalf("empty registration should keep the stored value, got %q", c.RegistrationNumber)
	}
}

func TestCompanyInputApplyReplacesRegistrationWhenGivenOnUpdate(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{RegistrationNumber: "REG-1"}
	in := CompanyInput{Name: "Bare Motors", Registration: "REG-2"}
	in.apply(&v, c, true)
	if err := v.Err(); err != nil {
		t.Fatalf("update with a new registration should validate: %v", err)
	}
	if c.RegistrationNumber != "REG-2" {
		t.Fatalf("a non-empty registration should replace the stored value, got %q", c.RegistrationNumber)
	}
}

func TestCompanyInputApplyRegistrationStaysEmptyOnCreate(t *testing.T) {
	var v apperr.Validation
	c := &model.Company{}
	in := CompanyInput{Name: "Bare Motors"}
	in.apply(&v, c, false)
	if err := v.Err(); err != nil {
		t.Fatalf("create without registration should validate: %v", err)
	}
	if c.RegistrationNumber != "" {
		t.Fatalf("create should not fabricate a registration, got %q", c.RegistrationNumber)
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
