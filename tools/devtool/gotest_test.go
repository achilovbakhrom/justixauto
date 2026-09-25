package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedPassword returns the same password every call, so expected argv/env
// strings in tests are deterministic.
func fixedPassword(pw string) func() (string, error) {
	return func() (string, error) { return pw, nil }
}

// alwaysReady is an httpProber stub that reports ready on the first attempt.
func alwaysReady(context.Context, string) (bool, error) { return true, nil }

// neverReady is an httpProber stub that never reports ready.
func neverReady(context.Context, string) (bool, error) { return false, nil }

func testCfg(pw string, probe httpProber) testConfig {
	return testConfig{
		pid:          4242,
		password:     fixedPassword(pw),
		maxAttempts:  3,
		attemptDelay: time.Millisecond,
		probe:        probe,
	}
}

// resultForPort makes the docker port fakeResult, e.g. for a container.
func resultForPort(port string) fakeResult {
	return fakeResult{output: port + "\n", exitCode: 0}
}

func TestRunTestArgvSequenceWithS3(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                    // docker run pg
		{exitCode: 0},                    // docker run s3
		resultForPort("127.0.0.1:9100"),  // docker port s3
		{exitCode: 0},                    // pg_isready
		resultForPort("127.0.0.1:15432"), // docker port pg
		{exitCode: 0},                    // go test
		{exitCode: 0},                    // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	if err := runTestWithConfig(context.Background(), a, nil, testCfg("deadbeef00112233445566778899aabb", alwaysReady)); err != nil {
		t.Fatalf("runTestWithConfig: %v", err)
	}

	if len(fr.calls) != 7 {
		t.Fatalf("got %d calls, want 7: %+v", len(fr.calls), fr.calls)
	}

	pgRun := fr.calls[0]
	wantPgRun := "docker run -d --rm --name justixauto-test-4242-pg -p 127.0.0.1::5432 " +
		"-e POSTGRES_PASSWORD=deadbeef00112233445566778899aabb -e POSTGRES_DB=justixauto_test " + testPGImage
	if argv(pgRun) != wantPgRun {
		t.Errorf("pg run argv = %q, want %q", argv(pgRun), wantPgRun)
	}

	s3Run := fr.calls[1]
	wantS3Run := "docker run -d --rm --name justixauto-test-4242-s3 -p 127.0.0.1::9000 " +
		"-e MINIO_ROOT_USER=justixtest -e MINIO_ROOT_PASSWORD=deadbeef00112233445566778899aabb " +
		testMinioImage + " server /data"
	if argv(s3Run) != wantS3Run {
		t.Errorf("s3 run argv = %q, want %q", argv(s3Run), wantS3Run)
	}

	if argv(fr.calls[2]) != "docker port justixauto-test-4242-s3 9000/tcp" {
		t.Errorf("s3 port argv = %q", argv(fr.calls[2]))
	}
	if argv(fr.calls[3]) != "docker exec justixauto-test-4242-pg pg_isready -U postgres -d justixauto_test -h 127.0.0.1" {
		t.Errorf("pg_isready argv = %q", argv(fr.calls[3]))
	}
	if argv(fr.calls[4]) != "docker port justixauto-test-4242-pg 5432/tcp" {
		t.Errorf("pg port argv = %q", argv(fr.calls[4]))
	}

	goTest := fr.calls[5]
	wantGoTest := "bash " + filepath.Join(root, "tools", "go.sh") + " test -race -count=1 -p 1 ./..."
	if argv(goTest) != wantGoTest {
		t.Errorf("go test argv = %q, want %q", argv(goTest), wantGoTest)
	}
	env := goTest.env
	if !containsEnv(env, "TEST_DATABASE_URL=postgres://postgres:deadbeef00112233445566778899aabb@127.0.0.1:15432/justixauto_test?sslmode=disable") {
		t.Errorf("env missing TEST_DATABASE_URL: %v", env)
	}
	if !containsEnv(env, "TEST_S3_ENDPOINT=http://127.0.0.1:9100") {
		t.Errorf("env missing TEST_S3_ENDPOINT: %v", env)
	}
	if !containsEnv(env, "AWS_ACCESS_KEY_ID=justixtest") ||
		!containsEnv(env, "AWS_SECRET_ACCESS_KEY=deadbeef00112233445566778899aabb") ||
		!containsEnv(env, "AWS_REGION=us-east-1") {
		t.Errorf("env missing AWS_* vars: %v", env)
	}

	cleanup := fr.calls[6]
	if argv(cleanup) != "docker rm -f justixauto-test-4242-pg justixauto-test-4242-s3" {
		t.Errorf("cleanup argv = %q", argv(cleanup))
	}
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func TestRunTestNoS3(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                    // docker run pg (no s3 run)
		{exitCode: 0},                    // pg_isready
		resultForPort("127.0.0.1:15432"), // docker port pg
		{exitCode: 0},                    // go test
		{exitCode: 0},                    // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	if err := runTestWithConfig(context.Background(), a, nil, testCfg("cafebabe00112233445566778899aabb", neverReady)); err != nil {
		t.Fatalf("runTestWithConfig: %v", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("got %d calls, want 5: %+v", len(fr.calls), fr.calls)
	}
	env := fr.calls[3].env
	for _, key := range []string{"TEST_S3_ENDPOINT=", "AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_REGION="} {
		for _, e := range env {
			if strings.HasPrefix(e, key) {
				t.Errorf("env should not contain %s: %v", key, env)
			}
		}
	}
	if argv(fr.calls[4]) != "docker rm -f justixauto-test-4242-pg justixauto-test-4242-s3" {
		t.Errorf("cleanup still targets both names, got %q", argv(fr.calls[4]))
	}
}

func TestRunTestForwardsArgsVerbatim(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0}, {exitCode: 0}, resultForPort("127.0.0.1:15432"), {exitCode: 0}, {exitCode: 0},
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	if err := runTestWithConfig(context.Background(), a, []string{"-run", "TestFoo", "./internal/..."}, testCfg("aa", neverReady)); err != nil {
		t.Fatalf("runTestWithConfig: %v", err)
	}
	got := fr.calls[3].args
	want := []string{filepath.Join(root, "tools", "go.sh"), "test", "-race", "-count=1", "-p", "1", "-run", "TestFoo", "./internal/..."}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("go test args = %v, want %v", got, want)
	}
}

func TestRunTestExitCodePropagated(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0}, {exitCode: 0}, resultForPort("127.0.0.1:15432"), {exitCode: 3}, {exitCode: 0},
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	var ee *exitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitError, got %T: %v", err, err)
	}
	if ee.code != 3 {
		t.Errorf("code = %d, want 3", ee.code)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("cleanup not called exactly once, got %d calls", len(fr.calls))
	}
}

func TestRunTestPGRunFails(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0, err: fmt.Errorf("exit status 125")}, // docker run pg fails
		{exitCode: 0}, // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	if err == nil || !strings.Contains(err.Error(), "start PostgreSQL container") {
		t.Fatalf("err = %v, want mention of 'start PostgreSQL container'", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected exactly one cleanup call after failure, got %d calls: %+v", len(fr.calls), fr.calls)
	}
}

func TestRunTestS3RunFails(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0}, // docker run pg ok
		{exitCode: 1}, // docker run s3 fails
		{exitCode: 0}, // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	if err == nil || !strings.Contains(err.Error(), "start MinIO container") {
		t.Fatalf("err = %v, want mention of 'start MinIO container'", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected go test not run, cleanup called once, got %d calls", len(fr.calls))
	}
}

func TestRunTestPostgresNeverReady(t *testing.T) {
	root := t.TempDir()
	results := []fakeResult{{exitCode: 0}} // docker run pg
	for i := 0; i < 3; i++ {
		results = append(results, fakeResult{exitCode: 1}) // pg_isready always fails
	}
	results = append(results, fakeResult{exitCode: 0}) // cleanup
	fr := &fakeRunner{t: t, results: results}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	if err == nil || err.Error() != "PostgreSQL test container not ready after 60s" {
		t.Fatalf("err = %v", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("expected go test not run, cleanup called once, got %d calls: %+v", len(fr.calls), fr.calls)
	}
}

func TestRunTestMinioNeverReady(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                   // docker run pg
		{exitCode: 0},                   // docker run s3
		resultForPort("127.0.0.1:9100"), // docker port s3
		{exitCode: 0},                   // pg_isready ready first try
		{exitCode: 0},                   // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	if err == nil || err.Error() != "MinIO test container not ready after 60s" {
		t.Fatalf("err = %v", err)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("expected go test not run, cleanup called once, got %d calls: %+v", len(fr.calls), fr.calls)
	}
}

func TestRunTestBadPortOutput(t *testing.T) {
	root := t.TempDir()
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                    // docker run pg
		{exitCode: 0},                    // pg_isready ready
		{output: "garbage", exitCode: 0}, // docker port pg: unparsable
		{exitCode: 0},                    // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg("aa", neverReady))
	if err == nil || !strings.Contains(err.Error(), "get PostgreSQL container port") {
		t.Fatalf("err = %v", err)
	}
	if len(fr.calls) != 4 {
		t.Fatalf("expected go test not run, cleanup called once, got %d calls: %+v", len(fr.calls), fr.calls)
	}
}

// cancelRunner cancels the given cancel func the Nth call it sees (0-indexed)
// and otherwise behaves like fakeRunner for calls before that.
type cancelRunner struct {
	fakeRunner
	cancel     context.CancelCauseFunc
	cancelAt   int
	cancelWith error
	calls      int
}

func (c *cancelRunner) run(ctx context.Context, cmd command) (string, int, error) {
	if c.calls == c.cancelAt {
		c.cancel(c.cancelWith)
		c.calls++
		return "", -1, c.cancelWith
	}
	c.calls++
	return c.fakeRunner.run(ctx, cmd)
}

func TestRunTestCancellationDuringGoTest(t *testing.T) {
	root := t.TempDir()
	inner := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                    // docker run pg
		{exitCode: 0},                    // pg_isready
		resultForPort("127.0.0.1:15432"), // docker port pg
		{},                               // go test: replaced by cancellation
		{exitCode: 0},                    // cleanup
	}}
	ctx, cancel := context.WithCancelCause(context.Background())
	sentinel := errors.New("boom: signal")
	cr := &cancelRunner{fakeRunner: *inner, cancel: cancel, cancelAt: 3, cancelWith: sentinel}
	a := &app{
		root: root, run: cr,
		stdout: &strings.Builder{}, stderr: &strings.Builder{},
		getenv:   func(k string) string { return map[string]string{"NO_S3": "1"}[k] },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(ctx, a, nil, testCfg("aa", neverReady))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel %v", err, sentinel)
	}
	if cr.calls != 5 {
		t.Fatalf("expected cleanup called once after cancellation, got %d calls", cr.calls)
	}
}

func TestRunTestPasswordNeverLogged(t *testing.T) {
	root := t.TempDir()
	pw := "5ecret5ecret5ecret5ecret5ecret5e"
	var stdout, stderr strings.Builder
	fr := &fakeRunner{t: t, results: []fakeResult{
		{exitCode: 0},                    // docker run pg
		{exitCode: 0},                    // docker run s3
		resultForPort("127.0.0.1:9100"),  // docker port s3
		{exitCode: 0},                    // pg_isready
		resultForPort("127.0.0.1:15432"), // docker port pg
		{output: "go test output, no secrets here\n", exitCode: 0}, // go test
		{exitCode: 0}, // cleanup
	}}
	a := &app{
		root: root, run: fr,
		stdout: &stdout, stderr: &stderr,
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	err := runTestWithConfig(context.Background(), a, nil, testCfg(pw, alwaysReady))
	if err != nil {
		t.Fatalf("runTestWithConfig: %v", err)
	}
	if strings.Contains(stdout.String(), pw) || strings.Contains(stderr.String(), pw) {
		t.Fatalf("password leaked into devtool output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	// Also check every captured call's returned output (what execRunner would
	// hand back for a discarded call) never carries the password.
	for _, r := range fr.results {
		if strings.Contains(r.output, pw) {
			t.Fatalf("password present in a call's captured output: %q", r.output)
		}
	}

	failErr := stepErr(context.Background(), "start PostgreSQL container", 1, nil)
	if strings.Contains(failErr.Error(), pw) {
		t.Fatalf("password leaked into a step error: %v", failErr)
	}
}

func TestParseContainerPort(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"ipv4", "127.0.0.1:49153\n", 49153, false},
		{"ipv6", "[::]:49153\n", 49153, false},
		{"two lines", "127.0.0.1:49153\n127.0.0.1:49154\n", 49153, false},
		{"empty", "", 0, true},
		{"no colon", "abc", 0, true},
		{"zero", "0.0.0.0:0", 0, true},
		{"too big", "0.0.0.0:70000", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseContainerPort(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got port %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestProbeHTTPReadyAndNotReady(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	notReady := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer notReady.Close()

	ok, err := probeHTTP(context.Background(), ready.URL+"/minio/health/ready")
	if err != nil || !ok {
		t.Errorf("ready server: ok=%v err=%v, want true, nil", ok, err)
	}
	ok, err = probeHTTP(context.Background(), notReady.URL+"/minio/health/ready")
	if err != nil || ok {
		t.Errorf("not-ready server: ok=%v err=%v, want false, nil", ok, err)
	}
}

func TestLookupCommandKnowsTest(t *testing.T) {
	if lookupCommand("test") == nil {
		t.Error(`lookupCommand("test") = nil`)
	}
}
