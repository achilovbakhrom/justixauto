package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoMod(t *testing.T, dir, module string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+module+"\n\ngo 1.27.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindRootNested(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root, "justixauto")
	nested := filepath.Join(root, "tools", "devtool")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findRoot(nested)
	if err != nil {
		t.Fatalf("findRoot: %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantResolved {
		t.Errorf("got %q, want %q", got, wantResolved)
	}
}

func TestFindRootSkipsForeignGoMod(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root, "justixauto")
	sub := filepath.Join(root, "tools", "lint")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoMod(t, sub, "justixauto-lint-tools")

	got, err := findRoot(sub)
	if err != nil {
		t.Fatalf("findRoot: %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantResolved {
		t.Errorf("got %q, want %q (should skip foreign go.mod)", got, wantResolved)
	}
}

func TestFindRootNotFound(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := findRoot(nested)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "go.mod of module justixauto not found in " + nested + " or its parents"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}
