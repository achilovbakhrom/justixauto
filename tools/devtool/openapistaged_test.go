package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func newOpenapiApp(fr *fakeRunner) *app {
	return &app{
		root:     "/repo",
		run:      fr,
		stdout:   &strings.Builder{},
		stderr:   &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", nil },
	}
}

func TestOpenapiStagedNoGoFiles(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "tools/devtool/main.go\nweb/x.ts\ninternal/a.go.txt\n", exitCode: 0},
	}}
	a := newOpenapiApp(fr)

	if err := runOpenapiStaged(context.Background(), a, nil); err != nil {
		t.Fatalf("runOpenapiStaged: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	if a.stdout.(*strings.Builder).String() != "" || a.stderr.(*strings.Builder).String() != "" {
		t.Errorf("expected no output, got stdout=%q stderr=%q", a.stdout, a.stderr)
	}
}

func TestOpenapiStagedDeletedFileCounts(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "cmd/x.go\n", exitCode: 0}, // staged deletion, still matches the path pattern
		{exitCode: 0},                       // unstaged check: clean
		{exitCode: 0},                       // make openapi
		{exitCode: 0},                       // spec diff: clean
		{exitCode: 0},                       // go diff: clean
	}}
	a := newOpenapiApp(fr)

	if err := runOpenapiStaged(context.Background(), a, nil); err != nil {
		t.Fatalf("runOpenapiStaged: %v", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("got %d calls, want 5: %+v", len(fr.calls), fr.calls)
	}
}

func TestOpenapiStagedUnstagedChangesBlocks(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 1}, // unstaged diff dirty
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("err = %v, want *exitError{1}", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2 (make must not run): %+v", len(fr.calls), fr.calls)
	}
	want := "pre-commit: stash or stage your unstaged Go changes so the OpenAPI spec matches the commit.\n"
	if a.stderr.(*strings.Builder).String() != want {
		t.Errorf("stderr = %q, want %q", a.stderr.(*strings.Builder).String(), want)
	}

	unstagedCall := fr.calls[1]
	if argv(unstagedCall) != "git diff --quiet -- cmd/*.go internal/*.go" {
		t.Errorf("unstaged diff argv = %q", argv(unstagedCall))
	}
}

func TestOpenapiStagedHappyPath(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 0}, // unstaged: clean
		{exitCode: 0}, // make openapi
		{exitCode: 0}, // spec diff: clean
		{exitCode: 0}, // go diff: clean
	}}
	a := newOpenapiApp(fr)

	if err := runOpenapiStaged(context.Background(), a, nil); err != nil {
		t.Fatalf("runOpenapiStaged: %v", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("got %d calls, want 5: %+v", len(fr.calls), fr.calls)
	}
	wantCalls := []string{
		"git diff --cached --name-only --diff-filter=ACMRD",
		"git diff --quiet -- cmd/*.go internal/*.go",
		"make --no-print-directory openapi",
		"git diff --quiet -- internal/pkg/apidocs/swagger.json",
		"git diff --quiet -- cmd/*.go internal/*.go",
	}
	for i, want := range wantCalls {
		if got := argv(fr.calls[i]); got != want {
			t.Errorf("call %d = %q, want %q", i, got, want)
		}
	}
	if a.stderr.(*strings.Builder).String() != "" {
		t.Errorf("expected no stderr, got %q", a.stderr)
	}
}

func TestOpenapiStagedSpecChanged(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 0}, // unstaged: clean
		{exitCode: 0}, // make openapi
		{exitCode: 1}, // spec diff: dirty -> short-circuits, no second diff
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("err = %v, want *exitError{1}", err)
	}
	if len(fr.calls) != 4 {
		t.Fatalf("got %d calls, want 4 (second diff short-circuited): %+v", len(fr.calls), fr.calls)
	}
	want := "pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them).\n" +
		"Review with 'git diff', then: git add internal/pkg/apidocs/swagger.json <changed files> && git commit\n"
	if a.stderr.(*strings.Builder).String() != want {
		t.Errorf("stderr = %q, want %q", a.stderr.(*strings.Builder).String(), want)
	}
}

func TestOpenapiStagedGoReformatted(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 0}, // unstaged: clean
		{exitCode: 0}, // make openapi
		{exitCode: 0}, // spec diff: clean
		{exitCode: 1}, // go diff: dirty (swag fmt reformatted)
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("err = %v, want *exitError{1}", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("got %d calls, want 5: %+v", len(fr.calls), fr.calls)
	}
	want := "pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them).\n" +
		"Review with 'git diff', then: git add internal/pkg/apidocs/swagger.json <changed files> && git commit\n"
	if a.stderr.(*strings.Builder).String() != want {
		t.Errorf("stderr = %q, want %q", a.stderr.(*strings.Builder).String(), want)
	}
}

func TestOpenapiStagedMakeFails(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 0},           // unstaged: clean
		{exitCode: 2, err: nil}, // make openapi fails
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	if err == nil || !strings.Contains(err.Error(), "run make openapi") {
		t.Fatalf("err = %v, want mention of 'run make openapi'", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("got %d calls, want 3 (spec diff must not run): %+v", len(fr.calls), fr.calls)
	}
	makeCall := fr.calls[2]
	if argv(makeCall) != "make --no-print-directory openapi" {
		t.Errorf("make argv = %q", argv(makeCall))
	}
}

func TestOpenapiStagedGitDiffCachedFails(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 128, err: errors.New("exit status 128")}, // not a git repository
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	if err == nil || !strings.Contains(err.Error(), "list staged files") {
		t.Fatalf("err = %v, want mention of 'list staged files'", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
}

func TestOpenapiStagedUnstagedDiffOtherExitCode(t *testing.T) {
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "internal/foo.go\n", exitCode: 0},
		{exitCode: 128, err: errors.New("exit status 128")},
	}}
	a := newOpenapiApp(fr)

	err := runOpenapiStaged(context.Background(), a, nil)
	if err == nil {
		t.Fatal("expected error for exit code other than 0/1")
	}
	var ee *exitError
	if errors.As(err, &ee) {
		t.Fatalf("expected a plain error (mapped to exit 1 by realMain), not *exitError, got %v", err)
	}
}

func TestLookupCommandKnowsOpenapiStaged(t *testing.T) {
	if lookupCommand("openapi-staged") == nil {
		t.Error(`lookupCommand("openapi-staged") = nil`)
	}
}
