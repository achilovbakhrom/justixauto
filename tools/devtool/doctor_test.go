package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func newDoctorApp(t *testing.T, present map[string]bool, results []fakeResult) (*app, *fakeRunner) {
	t.Helper()
	fr := &fakeRunner{t: t, results: results}
	a := &app{
		root:   t.TempDir(),
		run:    fr,
		stdout: &strings.Builder{},
		stderr: &strings.Builder{},
		getenv: func(string) string { return "" },
		lookPath: func(name string) (string, error) {
			if present[name] {
				return "/usr/bin/" + name, nil
			}
			return "", os.ErrNotExist
		},
	}
	return a, fr
}

func TestDoctorAllPass(t *testing.T) {
	present := map[string]bool{"git": true, "node": true, "npm": true, "docker": true}
	// order: go.sh version, node --version, npm --version, docker compose
	// version, docker info, git show-toplevel, git rev-parse HEAD
	results := []fakeResult{
		{exitCode: 0},
		{exitCode: 0},
		{exitCode: 0},
		{exitCode: 0},
		{exitCode: 0},
		{output: "", exitCode: 0}, // show-toplevel: replaced below per-test
		{exitCode: 0},
	}
	a, fr := newDoctorApp(t, present, results)
	// check-git needs its "detected root" call to equal a.root (resolved).
	resolved := a.root
	fr.results[5].output = resolved + "\n"

	err := runDoctor(context.Background(), a, nil)
	if err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	out := a.stdout.(*strings.Builder).String()
	if !strings.Contains(out, "FOUND: git") || !strings.Contains(out, "FOUND: docker") {
		t.Errorf("missing FOUND lines: %q", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "DOCTOR OK") {
		t.Errorf("expected trailing DOCTOR OK, got %q", out)
	}
	if strings.Contains(out, "codex") {
		t.Errorf("output must not mention codex: %q", out)
	}
	for _, c := range fr.calls {
		if c.name == "codex" || argv(c) == "codex" {
			t.Errorf("unexpected codex call: %+v", c)
		}
	}
}

func TestDoctorOneMissingSkipsVersionChecks(t *testing.T) {
	present := map[string]bool{"git": true, "node": false, "npm": true, "docker": true}
	results := []fakeResult{
		{exitCode: 0},             // go.sh version
		{exitCode: 0},             // npm --version (node skipped)
		{exitCode: 0},             // docker compose version
		{exitCode: 0},             // docker info
		{output: "", exitCode: 0}, // git show-toplevel
		{exitCode: 0},             // git rev-parse HEAD
	}
	a, fr := newDoctorApp(t, present, results)
	resolved := a.root
	fr.results[4].output = resolved + "\n"

	err := runDoctor(context.Background(), a, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("expected exitError{1} (MISSING: node itself fails the run), got %T %v", err, err)
	}
	out := a.stdout.(*strings.Builder).String()
	if !strings.Contains(out, "MISSING: node") {
		t.Errorf("expected MISSING: node, got %q", out)
	}
	for _, c := range fr.calls {
		if c.name == "node" {
			t.Errorf("node --version should have been skipped, got call %+v", c)
		}
	}
	if !strings.Contains(out, "DOCTOR FAILED: 1 check(s)") {
		t.Errorf("expected DOCTOR FAILED: 1 check(s) (remaining checks still ran and passed), got %q", out)
	}
	if !strings.Contains(out, "FOUND: git") || !strings.Contains(out, "FOUND: npm") || !strings.Contains(out, "FOUND: docker") {
		t.Errorf("remaining present checks should still run, got %q", out)
	}
}

func TestDoctorDaemonFailure(t *testing.T) {
	present := map[string]bool{"git": true, "node": true, "npm": true, "docker": true}
	results := []fakeResult{
		{exitCode: 0}, // go.sh version
		{exitCode: 0}, // node --version
		{exitCode: 0}, // npm --version
		{exitCode: 0}, // docker compose version
		{exitCode: 1}, // docker info (daemon unreachable)
		{output: "", exitCode: 0},
		{exitCode: 0},
	}
	a, fr := newDoctorApp(t, present, results)
	resolved := a.root
	fr.results[5].output = resolved + "\n"

	err := runDoctor(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected doctor to fail")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("expected exitError{1}, got %T %v", err, err)
	}
	out := a.stdout.(*strings.Builder).String()
	if !strings.Contains(out, "FAILED: docker info --format Docker daemon: {{.ServerVersion}}") {
		t.Errorf("expected FAILED line for docker info, got %q", out)
	}
	if !strings.Contains(out, "DOCTOR FAILED: 1 check(s)") {
		t.Errorf("expected DOCTOR FAILED: 1 check(s), got %q", out)
	}
}

func TestDoctorCheckGitFailure(t *testing.T) {
	present := map[string]bool{"git": true, "node": true, "npm": true, "docker": true}
	results := []fakeResult{
		{exitCode: 0},               // go.sh version
		{exitCode: 0},               // node --version
		{exitCode: 0},               // npm --version
		{exitCode: 0},               // docker compose version
		{exitCode: 0},               // docker info
		{output: "", exitCode: 128}, // git show-toplevel fails -> detected "none"
	}
	a, fr := newDoctorApp(t, present, results)
	_ = fr

	err := runDoctor(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected doctor to fail")
	}
	out := a.stdout.(*strings.Builder).String()
	if !strings.Contains(out, "FAILED: check-git") {
		t.Errorf("expected FAILED: check-git, got %q", out)
	}
	if !strings.Contains(out, "DOCTOR FAILED: 1 check(s)") {
		t.Errorf("expected DOCTOR FAILED: 1 check(s), got %q", out)
	}
}
