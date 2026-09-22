package database

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"justixauto/migrations"
)

// NewMigrator builds a migrator over the embedded SQL migrations.
// The caller must Close it.
func NewMigrator(url string) (*migrate.Migrate, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		db.Close()
		return nil, err
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		db.Close()
		return nil, err
	}
	return migrate.NewWithInstance("iofs", source, "pgx5", driver)
}
