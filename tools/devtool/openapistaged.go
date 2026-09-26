package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// openapiSpecPath matches tools/openapi-staged.sh's spec= variable.
const openapiSpecPath = "internal/pkg/apidocs/swagger.json"

// stagedGoPathRE matches tools/openapi-staged.sh's grep pattern for staged
// files that can affect the OpenAPI annotations.
var stagedGoPathRE = regexp.MustCompile(`^(cmd|internal)/.*\.go$`)

// runOpenapiStaged is the "openapi-staged" command-table entry (DT-07): a
// byte-exact Go port of tools/openapi-staged.sh's pre-commit check.
func runOpenapiStaged(ctx context.Context, a *app, _ []string) error {
	staged, err := stagedNames(ctx, a)
	if err != nil {
		return err
	}
	if !anyStagedGoPath(staged) {
		return nil
	}

	clean, err := diffQuiet(ctx, a, "check unstaged Go changes", "cmd/*.go", "internal/*.go")
	if err != nil {
		return err
	}
	if !clean {
		fmt.Fprintln(a.stderr, "pre-commit: stash or stage your unstaged Go changes so the OpenAPI spec matches the commit.")
		return &exitError{code: 1}
	}

	if err := runMakeOpenapi(ctx, a); err != nil {
		return err
	}

	specClean, err := diffQuiet(ctx, a, "check OpenAPI spec", openapiSpecPath)
	if err != nil {
		return err
	}
	goClean := true
	if specClean {
		goClean, err = diffQuiet(ctx, a, "check unstaged Go changes", "cmd/*.go", "internal/*.go")
		if err != nil {
			return err
		}
	}
	if !specClean || !goClean {
		fmt.Fprintln(a.stderr, "pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them).")
		fmt.Fprintln(a.stderr, "Review with 'git diff', then: git add "+openapiSpecPath+" <changed files> && git commit")
		return &exitError{code: 1}
	}
	return nil
}

// stagedNames runs `git diff --cached --name-only --diff-filter=ACMRD` and
// returns its output lines.
func stagedNames(ctx context.Context, a *app) ([]string, error) {
	out, code, err := a.run.run(ctx, command{
		name:   "git",
		args:   []string{"diff", "--cached", "--name-only", "--diff-filter=ACMRD"},
		dir:    a.root,
		stderr: a.stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("list staged files: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("list staged files: exit status %d", code)
	}
	return strings.Split(out, "\n"), nil
}

// anyStagedGoPath reports whether any staged path looks like it can affect
// the OpenAPI annotations (tools/openapi-staged.sh's grep -qE check).
func anyStagedGoPath(names []string) bool {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if stagedGoPathRE.MatchString(n) {
			return true
		}
	}
	return false
}

// diffQuiet runs `git diff --quiet -- <pathspecs>` and reports whether the
// working tree is clean for those pathspecs (exit 0) or dirty (exit 1). Any
// other outcome is an explicit error naming step. The pathspecs are passed
// as literal argv elements, never shell-expanded.
func diffQuiet(ctx context.Context, a *app, step string, pathspecs ...string) (bool, error) {
	args := append([]string{"diff", "--quiet", "--"}, pathspecs...)
	_, code, err := a.run.run(ctx, command{
		name:   "git",
		args:   args,
		dir:    a.root,
		stderr: a.stderr,
	})
	if err != nil {
		return false, fmt.Errorf("%s: %w", step, err)
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("%s: exit status %d", step, code)
	}
}

// runMakeOpenapi runs `make --no-print-directory openapi`, discarding its
// stdout (matching the base script's `>/dev/null`) and streaming its stderr.
func runMakeOpenapi(ctx context.Context, a *app) error {
	_, code, err := a.run.run(ctx, command{
		name:   "make",
		args:   []string{"--no-print-directory", "openapi"},
		dir:    a.root,
		stderr: a.stderr,
	})
	if err != nil {
		return fmt.Errorf("run make openapi: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("run make openapi: exit status %d", code)
	}
	return nil
}
