// Package architecture checks the dependency rules of the modular monolith:
//
//   - a module (internal/modules/<name>) never imports another module, the
//     composition root (internal/app) or the testkit;
//   - platform packages (internal/platform/...) never import modules or app.
//
// Only internal/app wires modules together, through ports the consuming
// module declares. This keeps the import graph a star around app, so module
// cycles cannot appear. Test files may import other modules (they consume
// the public API) and the e2e.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "justixauto/"

func TestImportRules(t *testing.T) {
	root := filepath.Join("..", "..")
	checked := 0
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkg := filepath.ToSlash(rel)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, imp := range f.Imports {
			target, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(target, module+"internal/") {
				continue
			}
			target = strings.TrimPrefix(target, module)
			if reason := violation(pkg, target); reason != "" {
				t.Errorf("%s imports %s: %s", filepath.ToSlash(path), target, reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d files checked; wrong root?", checked)
	}
}

// violation explains why pkg may not import target ("" if allowed).
func violation(pkg, target string) string {
	moduleOf := func(p string) string {
		if rest, ok := strings.CutPrefix(p, "internal/modules/"); ok {
			return strings.SplitN(rest, "/", 2)[0]
		}
		return ""
	}
	switch {
	case moduleOf(pkg) != "" && moduleOf(target) != "" && moduleOf(pkg) != moduleOf(target):
		return "modules must not import each other; declare a port and wire it in internal/app"
	case moduleOf(pkg) != "" && (target == "internal/app" || target == "internal/testkit"):
		return "modules must not depend on the composition root or the testkit"
	case strings.HasPrefix(pkg, "internal/platform") && (moduleOf(target) != "" || target == "internal/app"):
		return "platform packages must not depend on modules"
	}
	return ""
}

func TestViolationRules(t *testing.T) {
	cases := []struct {
		pkg, target string
		bad         bool
	}{
		{"internal/modules/commerce", "internal/modules/inventory", true},
		{"internal/modules/commerce", "internal/app", true},
		{"internal/modules/commerce", "internal/platform/auth", false},
		{"internal/modules/commerce/sub", "internal/modules/commerce", false},
		{"internal/platform/httpx", "internal/modules/identity", true},
		{"internal/app", "internal/modules/identity", false},
	}
	for _, c := range cases {
		if got := violation(c.pkg, c.target) != ""; got != c.bad {
			t.Errorf("%s -> %s: violation=%v", c.pkg, c.target, got)
		}
	}
}
