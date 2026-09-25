package main

import (
	"strings"
	"testing"
)

func TestRealMainUnknownCommand(t *testing.T) {
	var stdout, stderr strings.Builder
	code := realMain([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if stdout.String() != "" {
		t.Errorf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage on stderr, got %q", stderr.String())
	}
}

func TestRealMainNoArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	code := realMain(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage on stderr, got %q", stderr.String())
	}
}

func TestRealMainHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		var stdout, stderr strings.Builder
		code := realMain([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("%s: code = %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), "usage:") {
			t.Errorf("%s: expected usage on stdout, got %q", arg, stdout.String())
		}
		if stderr.String() != "" {
			t.Errorf("%s: expected no stderr, got %q", arg, stderr.String())
		}
	}
}

func TestRandomHex(t *testing.T) {
	h1, err := randomHex(16)
	if err != nil {
		t.Fatal(err)
	}
	if len(h1) != 32 {
		t.Errorf("len = %d, want 32", len(h1))
	}
	h2, err := randomHex(16)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("expected different values across calls")
	}
}

func TestLookupCommandKnowsP1Commands(t *testing.T) {
	for _, name := range []string{"check-git", "doctor", "env"} {
		if lookupCommand(name) == nil {
			t.Errorf("lookupCommand(%q) = nil", name)
		}
	}
	if lookupCommand("nope") != nil {
		t.Error("lookupCommand(\"nope\") should be nil")
	}
}
