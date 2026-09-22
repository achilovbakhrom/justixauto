package identity

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/database"
)

// openTestDB migrates and opens TEST_DATABASE_URL. It must point to a
// disposable database: the companies table is truncated.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	m, err := database.NewMigrator(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	m.Close()
	db, err := database.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("TRUNCATE identity.companies").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func newCompany(country, registration string) *Company {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &Company{ID: uuid.NewString(), Kind: KindSeller, Name: "Seller " + registration, Country: country,
		RegistrationNumber: registration, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
}

func TestCompanyRepositoryPostgres(t *testing.T) {
	ctx := context.Background()
	repo := NewCompanyRepository(openTestDB(t))

	c := newCompany("Uzbekistan", "100")
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, c.ID)
	if err != nil || got.Name != c.Name || !got.CreatedAt.Equal(c.CreatedAt) {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := repo.Get(ctx, uuid.NewString()); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := repo.Create(ctx, newCompany("UZBEKISTAN", "100")); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("case-insensitive duplicate: %v", err)
	}

	got.Name = "Renamed"
	if err := repo.Update(ctx, got, 1); err != nil || got.Version != 2 {
		t.Fatalf("update: %v version=%d", err, got.Version)
	}
	if err := repo.Update(ctx, got, 1); !errors.Is(err, apperr.ErrStale) {
		t.Fatalf("stale update: %v", err)
	}
	ghost := newCompany("Kazakhstan", "200")
	if err := repo.Update(ctx, ghost, 1); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	other := newCompany("Kazakhstan", "300")
	other.Kind = KindBank
	if err := repo.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	banks, err := repo.List(ctx, CompanyFilter{Kind: KindBank, Limit: 10})
	if err != nil || len(banks) != 1 || banks[0].ID != other.ID {
		t.Fatalf("list banks: %v %v", banks, err)
	}
	page, err := repo.List(ctx, CompanyFilter{Limit: 1, Offset: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("paging: %v %v", page, err)
	}
}
