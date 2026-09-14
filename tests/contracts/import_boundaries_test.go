package contracts_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These checks complement Go's type checker: the approved layout deliberately
// does not use internal directories. Scan source, including tests and inactive
// build tags, so a platform-specific adapter cannot hide an illegal dependency.
const modulePath = "justixauto"

type packageBoundary struct {
	area, owner, layer string
	public             bool
}

func classifyPackage(path string) (packageBoundary, error) {
	parts := strings.Split(path, "/")
	b := packageBoundary{area: parts[0]}
	if b.area != "services" {
		return b, nil
	}
	if len(parts) < 3 {
		return b, fmt.Errorf("owner packages must be under services/<owner>/<layer>: %s", path)
	}
	b.owner, b.layer = parts[1], parts[2]
	switch b.owner {
	case "identity", "inventory", "commerce", "retail", "financing", "insurance", "documents":
	default:
		return b, fmt.Errorf("unapproved service owner: %s", b.owner)
	}
	switch b.layer {
	case "domain", "app", "port", "adapter", "cmd":
	case "contracts":
		if len(parts) < 4 || (parts[3] != "openapi" && parts[3] != "events") {
			return b, fmt.Errorf("owner contracts must use contracts/openapi or contracts/events: %s", path)
		}
		b.public = true
	default:
		return b, fmt.Errorf("unapproved owner layer: %s", path)
	}
	return b, nil
}

func importViolation(source, imported string) string {
	from, err := classifyPackage(source)
	if err != nil {
		return err.Error()
	}
	// External library policy is version-locked separately. Here we enforce
	// repository ownership and dependency direction, not runtime DB permissions.
	if !strings.HasPrefix(imported, modulePath+"/") {
		return ""
	}
	to, err := classifyPackage(strings.TrimPrefix(imported, modulePath+"/"))
	if err != nil {
		return err.Error()
	}
	if from.area == "pkg" && (to.area == "services" || to.area == "edge") {
		return "shared mechanics cannot depend on an owner or edge"
	}
	if to.area == "services" && from.area != "services" {
		if from.area == "tests" || (from.area == "edge" && to.public) {
			return ""
		}
		return "owner implementation is private; consume its public contract"
	}
	if from.area != "services" {
		return ""
	}
	if to.area == "pkg" {
		return ""
	}
	if to.area != "services" {
		return "services may import only own layers, shared mechanics and public contracts"
	}
	if from.public {
		if to.public {
			return ""
		}
		return "public contracts cannot expose owner implementation dependencies"
	}
	if from.owner != to.owner {
		if to.public && from.layer != "domain" {
			return ""
		}
		return "cross-owner implementation/domain import is forbidden"
	}
	switch from.layer {
	case "domain":
		if to.layer != "domain" {
			return "domain may depend only on domain values and shared primitives"
		}
	case "app", "port":
		if to.layer == "adapter" || to.layer == "cmd" || (from.layer == "port" && to.layer == "app") {
			return "application and ports cannot depend on outer implementation layers"
		}
	case "adapter":
		if to.layer == "cmd" {
			return "adapters cannot depend on composition roots"
		}
	}
	return ""
}

func scanBoundaries(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				switch entry.Name() {
				case ".git", ".worktrees", "node_modules", "vendor", "docs", "testdata":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		source := filepath.ToSlash(filepath.Dir(relative))
		if _, err := classifyPackage(source); err != nil {
			violations = append(violations, relative+": "+err.Error())
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relative, err)
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if reason := importViolation(source, imported); reason != "" {
				violations = append(violations, fmt.Sprintf("%s imports %s: %s", relative, imported, reason))
			}
		}
		return nil
	})
	sort.Strings(violations)
	return violations, err
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate contract harness")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func TestRepositoryImportBoundaries(t *testing.T) {
	violations, err := scanBoundaries(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("architecture boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestImportBoundaryFixtures(t *testing.T) {
	cases := []struct {
		name, source, imported string
		forbidden              bool
	}{
		{"domain value", "services/retail/domain/deal", "services/retail/domain/customer", false},
		{"shared value", "services/retail/domain", "pkg/money", false},
		{"app command", "services/retail/app", "services/retail/domain/deal", false},
		{"app port", "services/retail/app", "services/retail/port", false},
		{"adapter", "services/retail/adapter/postgres", "services/retail/port", false},
		{"composition", "services/retail/cmd/api", "services/retail/adapter/http", false},
		{"foreign schema", "services/retail/adapter/http", "services/inventory/contracts/openapi/v1", false},
		{"foreign event", "services/retail/app", "services/inventory/contracts/events/v1", false},
		{"edge schema", "edge/app", "services/identity/contracts/openapi", false},
		{"integration test", "tests/integration", "services/retail/adapter/postgres", false},
		{"domain app", "services/retail/domain", "services/retail/app", true},
		{"domain adapter", "services/retail/domain", "services/retail/adapter/postgres", true},
		{"domain port", "services/retail/domain", "services/retail/port", true},
		{"app adapter", "services/retail/app", "services/retail/adapter/http", true},
		{"port app", "services/retail/port", "services/retail/app", true},
		{"foreign domain", "services/retail/app", "services/inventory/domain", true},
		{"foreign adapter", "services/retail/adapter/http", "services/inventory/adapter/http", true},
		{"foreign app", "services/retail/cmd/api", "services/commerce/app", true},
		{"foreign port", "services/retail/app", "services/commerce/port", true},
		{"shared owner", "pkg/eventstore", "services/retail/domain", true},
		{"shared contract", "pkg/events", "services/retail/contracts/events", true},
		{"shared edge", "pkg/auth", "edge/app", true},
		{"edge owner", "edge/app", "services/identity/app", true},
		{"service edge", "services/identity/app", "edge/app", true},
		{"invalid owner source", "services/finance/domain", "pkg/events", true},
		{"invalid owner target", "services/retail/app", "services/billing/contracts/openapi", true},
		{"invalid layer", "services/retail/repository", "pkg/events", true},
		{"contract leak", "services/retail/contracts/events", "services/retail/domain", true},
		{"unapproved public path", "services/retail/app", "services/inventory/contracts/private", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			// Blank imports and inactive build tags must still be checked.
			writeFixture(t, root, tc.source+"/fixture_test.go", "//go:build boundary_fixture\n\npackage fixture\nimport _ "+strconv.Quote(modulePath+"/"+tc.imported)+"\n")
			violations, err := scanBoundaries(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) > 0) != tc.forbidden {
				t.Fatalf("forbidden=%v; violations=%v", tc.forbidden, violations)
			}
		})
	}
}

func writeFixture(t *testing.T, root, name, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBoundaryTraversal(t *testing.T) {
	root := t.TempDir()
	for _, ignored := range []string{".git", ".worktrees/T-099", "node_modules", "vendor", "docs/justix-auto/dev/local/toolchains", "tests/contracts/testdata"} {
		writeFixture(t, root, ignored+"/invalid.go", "not Go source")
	}
	writeFixture(t, root, "services/identity/domain/value.go", "package domain\nimport \"time\"\n")
	violations, err := scanBoundaries(root)
	if err != nil || len(violations) != 0 {
		t.Fatalf("ignored trees and standard library: %v, %v", violations, err)
	}
	writeFixture(t, root, "services/finance/domain/invalid.go", "package domain\n")
	violations, err = scanBoundaries(root)
	if err != nil || len(violations) != 1 {
		t.Fatalf("invalid owner without imports: %v, %v", violations, err)
	}
	writeFixture(t, root, "services/retail/domain/broken.go", "not Go source")
	if _, err := scanBoundaries(root); err == nil {
		t.Fatal("malformed source must fail closed")
	}
}

func TestGoWrapperRejectsWrongCompiler(t *testing.T) {
	root := repositoryRoot(t)
	for _, version := range []string{"go1.27.0", "go1.28.0", "devel go1.27"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, "go")
			script := "#!/bin/sh\n[ \"$GOTOOLCHAIN\" = local ] || exit 91\n[ \"$1\" = env ] && [ \"$2\" = GOVERSION ] || exit 92\nprintf '%s\\n' '" + version + "'\n"
			if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", filepath.Join(root, "tools/go.sh"), "version")
			cmd.Env = append(os.Environ(), "JUSTIX_GO_BIN="+fake, "GOTOOLCHAIN=auto")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "must point to the installed go1.27.1") {
				t.Fatalf("wrong compiler accepted or wrong error: %v, %s", err, output)
			}
		})
	}
}

func TestGoWrapperExplicitCompilerAndArguments(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "go with spaces")
	script := `#!/bin/sh
[ "$GOTOOLCHAIN" = local ] || exit 91
if [ "$1" = env ] && [ "$2" = GOVERSION ]; then
  printf '%s\n' go1.27.1
  exit 0
fi
printf '<%s>\n' "$@"
`
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(repositoryRoot(t), "tools/go.sh"), "test", "argument with spaces", "./...")
	cmd.Dir = dir // Selection must not depend on the caller's current directory.
	cmd.Env = append(os.Environ(), "JUSTIX_GO_BIN="+fake, "GOTOOLCHAIN=auto")
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "<test>\n<argument with spaces>\n<./...>\n" {
		t.Fatalf("explicit compiler/environment/argument forwarding failed: %v, %s", err, output)
	}
}

func TestGoWrapperProjectToolchainDiscovery(t *testing.T) {
	wrapper, err := os.ReadFile(filepath.Join(repositoryRoot(t), "tools/go.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("linked=%v", linked), func(t *testing.T) {
			root := t.TempDir()
			mainRoot := filepath.Join(root, "main")
			checkout := mainRoot
			if linked {
				checkout = filepath.Join(root, "linked")
			}
			writeFixture(t, checkout, "tools/go.sh", string(wrapper))
			tool := "docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go"
			writeFixture(t, mainRoot, tool, "#!/bin/sh\nif [ \"$1\" = env ]; then printf '%s\\n' go1.27.1; else printf '%s\\n' found-project-toolchain; fi\n")
			if err := os.Chmod(filepath.Join(mainRoot, tool), 0o700); err != nil {
				t.Fatal(err)
			}
			// Mock only Git discovery, without initializing or mutating a repository.
			writeFixture(t, root, "bin/git", "#!/bin/sh\nprintf '%s\\n' \"$JUSTIX_FIXTURE_COMMON_DIR\"\n")
			if err := os.Chmod(filepath.Join(root, "bin/git"), 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", filepath.Join(checkout, "tools/go.sh"), "version")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "JUSTIX_GO_BIN=", "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "JUSTIX_FIXTURE_COMMON_DIR="+filepath.Join(mainRoot, ".git"))
			output, err := cmd.CombinedOutput()
			if err != nil || string(output) != "found-project-toolchain\n" {
				t.Fatalf("project toolchain discovery: %v, %s", err, output)
			}
		})
	}
}
