package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const allowedOrigins = "ALLOWED_ORIGINS=http://127.0.0.1:5173,http://127.0.0.1:5174,http://127.0.0.1:5175,http://127.0.0.1:5176"

// parsePort reads key from getenv (falling back to def), and requires it to
// parse as a decimal integer in [1, 65535].
func parsePort(getenv func(string) string, key, def string) (int, error) {
	v := getenv(key)
	if v == "" {
		v = def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid %s %q", key, v)
	}
	return n, nil
}

// transformEnv rewrites the .env.example template: POSTGRES_PASSWORD,
// DATABASE_URL, HTTP_ADDR and ALLOWED_ORIGINS lines are replaced wholesale;
// every other byte is unchanged. The result always ends with a single
// trailing newline, and POSTGRES_PORT is set by replacing an existing
// POSTGRES_PORT= line or, if none exists, appending one.
func transformEnv(template, password string, pgPort, apiPort int) string {
	lines := strings.Split(template, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	databaseURL := fmt.Sprintf(
		"DATABASE_URL=postgres://justixauto:%s@127.0.0.1:%d/justixauto?sslmode=disable",
		password, pgPort,
	)
	httpAddr := fmt.Sprintf("HTTP_ADDR=127.0.0.1:%d", apiPort)

	portLineFound := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "POSTGRES_PASSWORD="):
			lines[i] = "POSTGRES_PASSWORD=" + password
		case strings.HasPrefix(line, "DATABASE_URL="):
			lines[i] = databaseURL
		case strings.HasPrefix(line, "HTTP_ADDR="):
			lines[i] = httpAddr
		case strings.HasPrefix(line, "ALLOWED_ORIGINS="):
			lines[i] = allowedOrigins
		case strings.HasPrefix(line, "POSTGRES_PORT="):
			lines[i] = fmt.Sprintf("POSTGRES_PORT=%d", pgPort)
			portLineFound = true
		}
	}

	result := strings.Join(lines, "\n") + "\n"
	if !portLineFound {
		result += fmt.Sprintf("POSTGRES_PORT=%d\n", pgPort)
	}
	return result
}

func runEnv(_ context.Context, a *app, _ []string) error {
	pgPort, err := parsePort(a.getenv, "POSTGRES_PORT", "55432")
	if err != nil {
		return err
	}
	apiPort, err := parsePort(a.getenv, "API_PORT", "8080")
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(a.root)
	if err != nil {
		return fmt.Errorf("open project root: %w", err)
	}
	defer root.Close()

	template, err := root.ReadFile(".env.example")
	if err != nil {
		return fmt.Errorf("read .env.example: %w", err)
	}

	password, err := randomHex(16)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}

	content := transformEnv(string(template), password, pgPort, apiPort)

	f, err := root.OpenFile(".env", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintln(a.stdout, ".env exists — edit it or delete it first")
			return nil
		}
		return fmt.Errorf("create .env: %w", err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = root.Remove(".env")
		return fmt.Errorf("write .env: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(".env")
		return fmt.Errorf("write .env: %w", err)
	}

	fmt.Fprintf(a.stdout, "created .env (Postgres on %d, API on %d)\n", pgPort, apiPort)
	return nil
}
