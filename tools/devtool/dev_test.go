package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// devRunner is a concurrency-safe fake: every process blocks until its
// context is cancelled, except the one whose label exits on its own.
type devRunner struct {
	mu    sync.Mutex
	calls []command
	exit  string // argv substring of the process that exits by itself
	code  int
}

func (d *devRunner) run(ctx context.Context, c command) (string, int, error) {
	d.mu.Lock()
	d.calls = append(d.calls, c)
	d.mu.Unlock()
	if d.exit != "" && strings.Contains(argv(c), d.exit) {
		_, _ = c.stdout.Write([]byte("boom\n"))
		return "", d.code, nil
	}
	<-ctx.Done()
	return "", -1, context.Cause(ctx)
}

func (d *devRunner) env(t *testing.T, sub, key string) string {
	t.Helper()
	for _, c := range d.calls {
		if strings.Contains(argv(c), sub) {
			v := ""
			for _, e := range c.env {
				if strings.HasPrefix(e, key+"=") {
					v = strings.TrimPrefix(e, key+"=") // last one wins, as in os/exec
				}
			}
			return v
		}
	}
	t.Fatalf("no call matching %q", sub)
	return ""
}

func devTestApp(r runner, env map[string]string, busy ...int) (*app, *strings.Builder) {
	out := &strings.Builder{}
	return &app{
		root: "/repo", run: r, stdout: out, stderr: &strings.Builder{},
		getenv:   func(k string) string { return env[k] },
		portFree: func(p int) bool { return !contains(busy, p) },
	}, out
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestDevStartsAPIAndFourAppsAndStopsAllWhenOneExits(t *testing.T) {
	r := &devRunner{exit: "web/apps/insurance", code: 1}
	a, out := devTestApp(r, map[string]string{"HTTP_ADDR": "127.0.0.1:8090", "ALLOWED_ORIGINS": "http://x.test"})

	err := runDev(context.Background(), a, nil)
	if err == nil || !strings.Contains(err.Error(), "insurance exited with code 1") {
		t.Fatalf("err = %v, want insurance exit reported", err)
	}
	if len(r.calls) != 5 {
		t.Fatalf("got %d processes, want 5", len(r.calls))
	}
	if got := r.env(t, "cmd/api", "ALLOWED_ORIGINS"); got !=
		"http://x.test,http://127.0.0.1:5191,http://127.0.0.1:5192,http://127.0.0.1:5193,http://127.0.0.1:5194" {
		t.Errorf("API ALLOWED_ORIGINS = %q", got)
	}
	if got := r.env(t, "cmd/api", "WEB_DIR"); got != "" {
		t.Errorf("API WEB_DIR = %q, want empty (Vite serves the apps)", got)
	}
	if got := r.env(t, "web/apps/admin", "JUSTIX_API"); got != "http://127.0.0.1:8090" {
		t.Errorf("JUSTIX_API = %q", got)
	}
	for _, want := range []string{
		"npm run dev --workspace web/apps/realization -- --port 5191 --strictPort",
		"npm run dev --workspace web/apps/admin -- --port 5194 --strictPort",
		"bash /repo/tools/go.sh run ./cmd/api",
	} {
		found := false
		for _, c := range r.calls {
			found = found || argv(c) == want
		}
		if !found {
			t.Errorf("missing process %q", want)
		}
	}
	if s := out.String(); !strings.Contains(s, "admin        http://127.0.0.1:5194/admin/") ||
		!strings.Contains(s, "[insurance] boom\n") {
		t.Errorf("output missing URL or prefixed log line:\n%s", s)
	}
}

func TestDevSkipsBusyPortsWithoutSharingOne(t *testing.T) {
	ports, err := pickDevPorts(func(p int) bool { return p != 5191 && p != 5192 })
	if err != nil {
		t.Fatal(err)
	}
	// realization moves past 5191/5192 to 5193; financing must not reuse it.
	want := []int{5193, 5194, 5195, 5196}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports = %v, want %v", ports, want)
		}
	}
}

func TestDevDefaultsAPIAddressAndReturnsCauseOnCancel(t *testing.T) {
	r := &devRunner{}
	a, _ := devTestApp(r, map[string]string{})
	ctx, cancel := context.WithCancelCause(context.Background())
	stopped := errors.New("ctrl-c")
	done := make(chan error)
	go func() { done <- runDev(ctx, a, nil) }()
	for {
		r.mu.Lock()
		n := len(r.calls)
		r.mu.Unlock()
		if n == 5 {
			break
		}
	}
	cancel(stopped)
	if err := <-done; !errors.Is(err, stopped) {
		t.Fatalf("err = %v, want the cancel cause", err)
	}
	if got := r.env(t, "web/apps/realization", "JUSTIX_API"); got != "http://127.0.0.1:8080" {
		t.Errorf("JUSTIX_API = %q, want the default API address", got)
	}
}

func TestDevRejectsArguments(t *testing.T) {
	a, _ := devTestApp(&devRunner{}, nil)
	if err := runDev(context.Background(), a, []string{"x"}); err == nil {
		t.Fatal("want an error for unexpected arguments")
	}
}

func TestPrefixWriterKeepsLinesWholeAndFlushesTail(t *testing.T) {
	out := &strings.Builder{}
	w := &prefixWriter{w: out, prefix: "[a] ", mu: &sync.Mutex{}}
	_, _ = w.Write([]byte("one\ntw"))
	_, _ = w.Write([]byte("o\nthree"))
	w.flush()
	if got := out.String(); got != "[a] one\n[a] two\n[a] three\n" {
		t.Fatalf("got %q", got)
	}
}
