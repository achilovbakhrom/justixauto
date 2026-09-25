package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// moduleName returns the module directive's argument from a go.mod file's
// contents, or "" if none is found. Only the first "module " line matters.
func moduleName(data []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// isProjectRoot reports whether dir directly contains a go.mod whose module
// directive is exactly "justixauto".
func isProjectRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // dir walks only real filesystem ancestors of the process cwd; not user input
	if err != nil {
		return false
	}
	return moduleName(data) == "justixauto"
}

// findRoot walks up from cwd (inclusive) looking for the nearest ancestor
// containing a go.mod whose module directive is exactly "justixauto",
// resolving symlinks first (D-5). go.mod files belonging to other modules
// (e.g. tools/lint/go.mod) are not considered, since only the directory's own
// go.mod is checked at each level.
func findRoot(cwd string) (string, error) {
	notFound := fmt.Errorf("go.mod of module justixauto not found in %s or its parents", cwd)

	dir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", notFound
	}
	for {
		if isProjectRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", notFound
		}
		dir = parent
	}
}
