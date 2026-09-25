package main

import (
	"context"
	"strings"
	"testing"
)

// fakeRunner lets tests script expected argv sequences and their results
// without starting real subprocesses. If a call's command has a non-nil
// stdout, the fake writes that call's output to it, mirroring how the real
// execRunner streams output instead of capturing it.
type fakeRunner struct {
	calls   []command
	results []fakeResult
	t       *testing.T
}

type fakeResult struct {
	output   string
	exitCode int
	err      error
}

func (f *fakeRunner) run(_ context.Context, c command) (string, int, error) {
	f.calls = append(f.calls, c)
	idx := len(f.calls) - 1
	if idx >= len(f.results) {
		if f.t != nil {
			f.t.Fatalf("unexpected call #%d: %+v", idx, c)
		}
		return "", 0, nil
	}
	r := f.results[idx]
	if c.stdout != nil {
		_, _ = c.stdout.Write([]byte(r.output))
		return "", r.exitCode, r.err
	}
	return r.output, r.exitCode, r.err
}

func argv(c command) string {
	return strings.TrimSpace(c.name + " " + strings.Join(c.args, " "))
}

func TestExecRunnerRunsAndReportsExit(t *testing.T) {
	r := execRunner{}
	_, code, err := r.run(context.Background(), command{name: "true"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}

	_, code, err = r.run(context.Background(), command{name: "false"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
}

func TestExecRunnerNoSuchProgram(t *testing.T) {
	r := execRunner{}
	_, code, err := r.run(context.Background(), command{name: "devtool-does-not-exist-xyz"})
	if err == nil {
		t.Fatal("expected error for missing program")
	}
	if code != -1 {
		t.Errorf("code = %d, want -1", code)
	}
}

func TestExecRunnerCancelledContext(t *testing.T) {
	r := execRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, code, err := r.run(ctx, command{name: "sleep", args: []string{"1"}})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if code != -1 {
		t.Errorf("code = %d, want -1", code)
	}
}
