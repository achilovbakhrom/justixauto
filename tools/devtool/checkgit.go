package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// detectGitRoot returns the trimmed stdout of `git -C <root> rev-parse
// --show-toplevel`, or "" if git is missing or the command fails.
func detectGitRoot(ctx context.Context, a *app) string {
	out, code, err := a.run.run(ctx, command{
		name: "git",
		args: []string{"-C", a.root, "rev-parse", "--show-toplevel"},
		dir:  a.root,
	})
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// checkGit reproduces tools/check-git.sh: it writes the same stdout lines to
// a.stdout and returns the same exit code (0, 4 or 5). Git stderr is
// discarded, matching the shell script.
func checkGit(ctx context.Context, a *app) int {
	detected := detectGitRoot(ctx, a)
	displayDetected := detected
	if displayDetected == "" {
		displayDetected = "none"
	}

	rootResolved, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		rootResolved = a.root
	}
	detectedResolved := detected
	if detected != "" {
		if r, err := filepath.EvalSymlinks(detected); err == nil {
			detectedResolved = r
		}
	}

	if detectedResolved != rootResolved {
		fmt.Fprintf(a.stdout, "GIT CHECK FAILED: project needs its own Git repository: %s\n", a.root)
		fmt.Fprintf(a.stdout, "Detected Git root: %s. Do not use the parent repository.\n", displayDetected)
		fmt.Fprintf(a.stdout, "User action: cd '%s' && git init -b main && git add . && git commit -m 'Prepare development workspace'\n", a.root)
		return 4
	}

	_, code, err := a.run.run(ctx, command{
		name: "git",
		args: []string{"-C", a.root, "rev-parse", "--verify", "HEAD"},
		dir:  a.root,
	})
	if err != nil || code != 0 {
		fmt.Fprintln(a.stdout, "GIT CHECK FAILED: create the first commit before development.")
		return 5
	}

	fmt.Fprintf(a.stdout, "GIT CHECK OK: %s\n", a.root)
	return 0
}

func runCheckGit(ctx context.Context, a *app, _ []string) error {
	code := checkGit(ctx, a)
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}
