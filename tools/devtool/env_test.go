package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureTemplate = `# Local development only. Copy to .env (git-ignored) and adjust.
POSTGRES_PASSWORD=change-me-local-only
DATABASE_URL=postgres://justixauto:change-me-local-only@127.0.0.1:55432/justixauto?sslmode=disable
HTTP_ADDR=127.0.0.1:8080
# Plain-HTTP local development only; keep true (default) everywhere else.
COOKIE_SECURE=false
ALLOWED_ORIGINS=http://127.0.0.1:5173,http://127.0.0.1:5174,http://127.0.0.1:5175,http://127.0.0.1:5176
WEB_DIR=web/apps
`

func newEnvApp(t *testing.T, getenv map[string]string) (*app, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte(fixtureTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &app{
		root:   root,
		run:    &fakeRunner{t: t},
		stdout: &strings.Builder{},
		stderr: &strings.Builder{},
		getenv: func(k string) string { return getenv[k] },
		lookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	}
	return a, root
}

func TestEnvDefaultPorts(t *testing.T) {
	a, root := newEnvApp(t, nil)
	if err := runEnv(context.Background(), a, nil); err != nil {
		t.Fatalf("runEnv: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "127.0.0.1:55432/justixauto") {
		t.Errorf("expected default Postgres port 55432 in DATABASE_URL: %q", content)
	}
	if !strings.Contains(content, "HTTP_ADDR=127.0.0.1:8080") {
		t.Errorf("expected default API port 8080: %q", content)
	}
	if !strings.Contains(content, "POSTGRES_PORT=55432") {
		t.Errorf("expected appended POSTGRES_PORT=55432: %q", content)
	}
	out := a.stdout.(*strings.Builder).String()
	if out != "created .env (Postgres on 55432, API on 8080)\n" {
		t.Errorf("unexpected stdout: %q", out)
	}
}

func TestEnvCustomPorts(t *testing.T) {
	a, root := newEnvApp(t, map[string]string{"POSTGRES_PORT": "55499", "API_PORT": "8099"})
	if err := runEnv(context.Background(), a, nil); err != nil {
		t.Fatalf("runEnv: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "127.0.0.1:55499/justixauto") {
		t.Errorf("expected custom Postgres port 55499 in DATABASE_URL: %q", content)
	}
	if !strings.Contains(content, "HTTP_ADDR=127.0.0.1:8099") {
		t.Errorf("expected custom API port 8099: %q", content)
	}
	if !strings.Contains(content, "POSTGRES_PORT=55499") {
		t.Errorf("expected appended POSTGRES_PORT=55499: %q", content)
	}
}

func TestEnvInvalidPort(t *testing.T) {
	a, root := newEnvApp(t, map[string]string{"POSTGRES_PORT": "not-a-port"})
	err := runEnv(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	want := `invalid POSTGRES_PORT "not-a-port"`
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".env")); statErr == nil {
		t.Error(".env must not be created on invalid port")
	}
}

func TestEnvOutOfRangePort(t *testing.T) {
	a, _ := newEnvApp(t, map[string]string{"API_PORT": "70000"})
	err := runEnv(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	want := `invalid API_PORT "70000"`
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

func TestEnvExistingUntouched(t *testing.T) {
	a, root := newEnvApp(t, nil)
	existing := "POSTGRES_PASSWORD=already-here\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runEnv(context.Background(), a, nil); err != nil {
		t.Fatalf("runEnv: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existing {
		t.Errorf(".env was modified: %q", string(data))
	}
	out := a.stdout.(*strings.Builder).String()
	if out != ".env exists — edit it or delete it first\n" {
		t.Errorf("unexpected stdout: %q", out)
	}
}

func TestEnvMissingTemplate(t *testing.T) {
	root := t.TempDir()
	a := &app{
		root:     root,
		run:      &fakeRunner{t: t},
		stdout:   &strings.Builder{},
		stderr:   &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
	err := runEnv(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected error for missing .env.example")
	}
	if _, statErr := os.Stat(filepath.Join(root, ".env")); statErr == nil {
		t.Error(".env must not be created when template is missing")
	}
}

func TestEnvFileMode(t *testing.T) {
	a, root := newEnvApp(t, nil)
	if err := runEnv(context.Background(), a, nil); err != nil {
		t.Fatalf("runEnv: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestEnvPasswordDiffersAcrossRuns(t *testing.T) {
	a1, root1 := newEnvApp(t, nil)
	a2, root2 := newEnvApp(t, nil)
	if err := runEnv(context.Background(), a1, nil); err != nil {
		t.Fatal(err)
	}
	if err := runEnv(context.Background(), a2, nil); err != nil {
		t.Fatal(err)
	}
	data1, err := os.ReadFile(filepath.Join(root1, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	data2, err := os.ReadFile(filepath.Join(root2, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	pw1 := extractPassword(t, string(data1))
	pw2 := extractPassword(t, string(data2))
	if len(pw1) != 32 {
		t.Errorf("password length = %d, want 32", len(pw1))
	}
	if pw1 == pw2 {
		t.Error("expected different passwords across runs")
	}
}

func extractPassword(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "POSTGRES_PASSWORD=") {
			return strings.TrimPrefix(line, "POSTGRES_PASSWORD=")
		}
	}
	t.Fatal("POSTGRES_PASSWORD line not found")
	return ""
}

func TestTransformEnvContent(t *testing.T) {
	got := transformEnv(fixtureTemplate, "deadbeef00000000000000000000000", 55432, 8080)

	lines := strings.Split(got, "\n")
	if lines[0] != "# Local development only. Copy to .env (git-ignored) and adjust." {
		t.Errorf("comment line changed: %q", lines[0])
	}
	if !strings.Contains(got, "COOKIE_SECURE=false") {
		t.Errorf("unrelated line COOKIE_SECURE must be untouched: %q", got)
	}
	if !strings.Contains(got, "WEB_DIR=web/apps") {
		t.Errorf("unrelated line WEB_DIR must be untouched: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("result must end with a newline: %q", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("result must not have a blank trailing line: %q", got)
	}
	if !strings.Contains(got, "POSTGRES_PORT=55432") {
		t.Errorf("expected appended POSTGRES_PORT: %q", got)
	}
}

func TestTransformEnvReplacesExistingPortLine(t *testing.T) {
	template := fixtureTemplate + "POSTGRES_PORT=1234\n"
	got := transformEnv(template, "pw", 55432, 8080)
	if strings.Count(got, "POSTGRES_PORT=") != 1 {
		t.Fatalf("expected exactly one POSTGRES_PORT= line: %q", got)
	}
	if !strings.Contains(got, "POSTGRES_PORT=55432") {
		t.Errorf("expected existing POSTGRES_PORT= line replaced: %q", got)
	}
}

func TestTransformEnvNoTrailingNewlineInTemplate(t *testing.T) {
	template := "POSTGRES_PASSWORD=x"
	got := transformEnv(template, "pw", 1, 2)
	want := "POSTGRES_PASSWORD=pw\nPOSTGRES_PORT=1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
