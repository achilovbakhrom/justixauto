// Command devtool replaces the tools/*.sh scripts with a single Go program.
// Invoke it as `bash tools/go.sh run ./tools/devtool <command> [args...]`.
// Note: `go run` always reports a failing subcommand's exit code as 1 (with an
// "exit status N" line on stderr); callers that need the real exit code build
// the binary first (see tools/check-git.sh).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// app bundles the dependencies a subcommand needs, so tests can inject fakes.
type app struct {
	root     string
	run      runner
	stdout   io.Writer
	stderr   io.Writer
	getenv   func(string) string
	lookPath func(string) (string, error)
}

// exitError requests a specific process exit code without printing anything
// beyond what the command already wrote.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

// sigCause is the context.Cause recorded when a signal cancels realMain's
// context.
type sigCause struct{ sig os.Signal }

func (s sigCause) Error() string { return fmt.Sprintf("received signal: %v", s.sig) }

var commands = []struct {
	name, summary string
	run           func(ctx context.Context, a *app, args []string) error
}{
	{"check-git", "verify the project has its own Git repository with a commit", runCheckGit},
	{"doctor", "check local development prerequisites", runDoctor},
	{"env", "create .env from .env.example with generated secrets", runEnv},
	{"test", "run go test with throwaway PostgreSQL/MinIO containers", runTest},
	{"openapi-staged", "regenerate and verify the OpenAPI spec against staged Go changes", runOpenapiStaged},
}

// randomHex returns 16 bytes of crypto/rand entropy as 32 lowercase hex
// characters: the generated password shape shared by env and test.
func randomHex() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: bash tools/go.sh run ./tools/devtool <command> [args...]")
	fmt.Fprintln(w, "commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-16s %s\n", c.name, c.summary)
	}
}

func lookupCommand(name string) func(ctx context.Context, a *app, args []string) error {
	for _, c := range commands {
		if c.name == name {
			return c.run
		}
	}
	return nil
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	}

	cmdName := args[0]
	cmdFunc := lookupCommand(cmdName)
	if cmdFunc == nil {
		printUsage(stderr)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "devtool: %s\n", err)
		return 1
	}
	root, err := findRoot(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "devtool: %s\n", err)
		return 1
	}

	a := &app{
		root:     root,
		run:      execRunner{},
		stdout:   stdout,
		stderr:   stderr,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case sig, ok := <-sigCh:
			if !ok {
				return
			}
			signal.Stop(sigCh)
			cancel(sigCause{sig})
		case <-done:
			signal.Stop(sigCh)
		}
	}()

	runErr := cmdFunc(ctx, a, args[1:])
	cancel(nil)

	var ee *exitError
	if errors.As(runErr, &ee) {
		return ee.code
	}
	if runErr == nil {
		return 0
	}

	var sc sigCause
	if errors.As(context.Cause(ctx), &sc) {
		if sc.sig == syscall.SIGTERM {
			return 143
		}
		return 130
	}

	fmt.Fprintf(stderr, "devtool %s: %s\n", cmdName, runErr)
	return 1
}
