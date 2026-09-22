// Command bootstrap-admin creates the first platform administrator. It is
// single-use: it refuses once any active platform administrator exists. The
// password is read from standard input, never from flags or the environment.
//
//	printf '%s' "$PASSWORD" | bootstrap-admin -login admin -email admin@example.com -name "Admin"
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"justixauto/internal/config"
	"justixauto/internal/modules/identity"
	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap-admin:", err)
		var v *apperr.ValidationError
		if errors.As(err, &v) {
			for field, msg := range v.Fields {
				fmt.Fprintf(os.Stderr, "  %s: %s\n", field, msg)
			}
		}
		os.Exit(1)
	}
}

func run() error {
	login := flag.String("login", "", "sign-in login")
	email := flag.String("email", "", "email address")
	name := flag.String("name", "", "display name")
	flag.Parse()
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		return errors.New("password must be provided on standard input")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	m, err := identity.New(db, identity.Config{Session: identity.DefaultSessionConfig, MFAKey: cfg.MFAKey})
	if err != nil {
		return err
	}
	u, err := m.Users.Bootstrap(context.Background(), identity.BootstrapInput{
		DisplayName: *name, Login: *login, Email: *email, Password: strings.TrimRight(password, "\r\n"),
	})
	if err != nil {
		return err
	}
	fmt.Printf("created platform administrator %s (%s)\n", *u.Login, u.ID)
	return nil
}
