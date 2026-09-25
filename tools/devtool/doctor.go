package main

import (
	"context"
	"fmt"
	"strings"
)

// runDoctor reproduces tools/doctor.sh, minus the codex check and the
// "not scaffolded" line (D-11): every check runs even after a failure, and a
// missing program's version check is skipped rather than attempted.
func runDoctor(ctx context.Context, a *app, _ []string) error {
	failed := 0

	present := map[string]bool{}
	for _, p := range []string{"git", "node", "npm", "docker"} {
		if _, err := a.lookPath(p); err != nil {
			fmt.Fprintf(a.stdout, "MISSING: %s\n", p)
			present[p] = false
			failed++
		} else {
			fmt.Fprintf(a.stdout, "FOUND: %s\n", p)
			present[p] = true
		}
	}

	runStep := func(name string, args ...string) bool {
		_, code, err := a.run.run(ctx, command{
			name:   name,
			args:   args,
			dir:    a.root,
			stdout: a.stdout,
		})
		if err != nil || code != 0 {
			fmt.Fprintf(a.stdout, "FAILED: %s\n", strings.Join(append([]string{name}, args...), " "))
			return false
		}
		return true
	}

	if !runStep("bash", "tools/go.sh", "version") {
		failed++
	}
	if present["node"] && !runStep("node", "--version") {
		failed++
	}
	if present["npm"] && !runStep("npm", "--version") {
		failed++
	}
	if present["docker"] {
		if !runStep("docker", "compose", "version") {
			failed++
		}
		if !runStep("docker", "info", "--format", "Docker daemon: {{.ServerVersion}}") {
			failed++
		}
	}

	if code := checkGit(ctx, a); code != 0 {
		fmt.Fprintln(a.stdout, "FAILED: check-git")
		failed++
	}

	if failed == 0 {
		fmt.Fprintln(a.stdout, "DOCTOR OK")
		return nil
	}
	fmt.Fprintf(a.stdout, "DOCTOR FAILED: %d check(s)\n", failed)
	return &exitError{code: 1}
}
