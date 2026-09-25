package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// command describes a single subprocess invocation. name is looked up on
// PATH (never passed through a shell); args is the argv tail.
type command struct {
	name   string
	args   []string
	dir    string    // always the project root
	env    []string  // nil = inherit os.Environ(); otherwise the full environment
	stdout io.Writer // nil = capture and return as output
	stderr io.Writer // nil = discard
}

// runner starts a subprocess and reports how it ended. err is non-nil only
// when the process could not be started or ctx was cancelled before/while it
// ran; in either case exitCode is -1.
type runner interface {
	run(ctx context.Context, c command) (output string, exitCode int, err error)
}

// execRunner runs commands with os/exec. Argv always comes from the fixed
// command table built in this package; no shell is ever invoked.
type execRunner struct{}

func (execRunner) run(ctx context.Context, c command) (string, int, error) {
	cmd := exec.CommandContext(ctx, c.name, c.args...) //nolint:gosec // argv from the fixed command table; no shell
	cmd.Dir = c.dir
	if c.env != nil {
		cmd.Env = c.env
	}
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 10 * time.Second

	var buf strings.Builder
	if c.stdout != nil {
		cmd.Stdout = c.stdout
	} else {
		cmd.Stdout = &buf
	}
	if c.stderr != nil {
		cmd.Stderr = c.stderr
	}

	runErr := cmd.Run()

	if ctx.Err() != nil {
		return buf.String(), -1, context.Cause(ctx)
	}
	if cmd.ProcessState == nil {
		return buf.String(), -1, runErr
	}
	return buf.String(), cmd.ProcessState.ExitCode(), nil
}
