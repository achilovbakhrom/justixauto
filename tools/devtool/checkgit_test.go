package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T, root string, run runner) *app {
	t.Helper()
	return &app{
		root:     root,
		run:      run,
		stdout:   &strings.Builder{},
		stderr:   &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
}

func TestCheckGitOK(t *testing.T) {
	root := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: resolvedRoot + "\n", exitCode: 0},
		{output: "", exitCode: 0},
	}}
	a := newTestApp(t, resolvedRoot, fr)

	code := checkGit(context.Background(), a)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	got := a.stdout.(*strings.Builder).String()
	want := "GIT CHECK OK: " + resolvedRoot + "\n"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestCheckGitForeignRoot(t *testing.T) {
	root := t.TempDir()
	foreign := t.TempDir()
	resolvedForeign, err := filepath.EvalSymlinks(foreign)
	if err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: resolvedForeign + "\n", exitCode: 0},
	}}
	a := newTestApp(t, root, fr)

	code := checkGit(context.Background(), a)
	if code != 4 {
		t.Fatalf("code = %d, want 4", code)
	}
	got := a.stdout.(*strings.Builder).String()
	want := "GIT CHECK FAILED: project needs its own Git repository: " + root + "\n" +
		"Detected Git root: " + resolvedForeign + ". Do not use the parent repository.\n" +
		"User action: cd '" + root + "' && git init -b main && git add . && git commit -m 'Prepare development workspace'\n"
	if got != want {
		t.Errorf("stdout =\n%q\nwant\n%q", got, want)
	}
}

func TestCheckGitFailurePrintsNone(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "", exitCode: 128, err: nil},
	}}
	a := newTestApp(t, root, fr)

	code := checkGit(context.Background(), a)
	if code != 4 {
		t.Fatalf("code = %d, want 4", code)
	}
	got := a.stdout.(*strings.Builder).String()
	if !strings.Contains(got, "Detected Git root: none.") {
		t.Errorf("stdout %q missing 'Detected Git root: none.'", got)
	}
}

func TestCheckGitNoHead(t *testing.T) {
	root := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: resolvedRoot + "\n", exitCode: 0},
		{output: "", exitCode: 128},
	}}
	a := newTestApp(t, resolvedRoot, fr)

	code := checkGit(context.Background(), a)
	if code != 5 {
		t.Fatalf("code = %d, want 5", code)
	}
	got := a.stdout.(*strings.Builder).String()
	want := "GIT CHECK FAILED: create the first commit before development.\n"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunCheckGitExitError(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{output: "", exitCode: 128},
	}}
	a := newTestApp(t, root, fr)

	err := runCheckGit(context.Background(), a, nil)
	var ee *exitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitError, got %T: %v", err, err)
	}
	if ee.code != 4 {
		t.Errorf("code = %d, want 4", ee.code)
	}
}
