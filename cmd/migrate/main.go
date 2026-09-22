// Command migrate applies or rolls back the embedded SQL migrations.
//
//	migrate up          apply all pending migrations
//	migrate down [N]    roll back N migrations (default 1)
//	migrate version     print the current version
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"

	"justixauto/internal/platform/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: migrate up | down [N] | version")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	m, err := database.NewMigrator(url)
	if err != nil {
		return err
	}
	defer m.Close()

	switch args[0] {
	case "up":
		err = m.Up()
	case "down":
		steps := 1
		if len(args) > 1 {
			if steps, err = strconv.Atoi(args[1]); err != nil || steps < 1 {
				return errors.New("down: N must be a positive number")
			}
		}
		err = m.Steps(-steps)
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil && !errors.Is(verr, migrate.ErrNilVersion) {
			return verr
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return nil
	}
	return err
}
