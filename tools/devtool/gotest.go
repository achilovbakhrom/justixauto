package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Image digests match tools/test-go.sh exactly (T-DEVTOOL P2).
const (
	testPGImage    = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	testMinioImage = "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
)

// containerNames holds the throwaway container names derived from the
// devtool process's PID, mirroring tools/test-go.sh's "justixauto-test-$$".
type containerNames struct {
	pg, s3 string
}

func newContainerNames(pid int) containerNames {
	base := fmt.Sprintf("justixauto-test-%d", pid)
	return containerNames{pg: base + "-pg", s3: base + "-s3"}
}

// httpProber checks a readiness endpoint; injected so tests can avoid real
// containers and real sleeps.
type httpProber func(ctx context.Context, url string) (bool, error)

// probeHTTP is the production httpProber: a single GET with a 2s timeout.
func probeHTTP(ctx context.Context, url string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK, nil
}

// testConfig bundles the values P2 must be able to inject in tests: the pid
// used to derive container names, the password source, the readiness
// polling budget, and the HTTP prober.
type testConfig struct {
	pid          int
	password     func() (string, error)
	maxAttempts  int
	attemptDelay time.Duration
	probe        httpProber
}

func defaultTestConfig() testConfig {
	return testConfig{
		pid:          os.Getpid(),
		password:     randomHex,
		maxAttempts:  60,
		attemptDelay: time.Second,
		probe:        probeHTTP,
	}
}

// runTest is the "test" command-table entry (DT-06): throwaway PostgreSQL
// (and, unless NO_S3=1, MinIO) containers for `go test -race -count=1 -p 1`.
func runTest(ctx context.Context, a *app, args []string) error {
	return runTestWithConfig(ctx, a, args, defaultTestConfig())
}

func runTestWithConfig(ctx context.Context, a *app, args []string, cfg testConfig) error {
	password, err := cfg.password()
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	s3 := a.getenv("NO_S3") != "1"
	names := newContainerNames(cfg.pid)

	// Registered before the first container starts; runs on every return
	// path (success, error, cancellation) with a fresh, timed-out context.
	defer cleanupContainers(ctx, a, names)

	if err := startPostgres(ctx, a, names, password); err != nil {
		return err
	}
	endpoint, err := startMinio(ctx, a, names, password, s3)
	if err != nil {
		return err
	}
	if err := waitPostgresReady(ctx, a, cfg, names); err != nil {
		return err
	}
	if err := waitMinioReady(ctx, cfg, endpoint, s3); err != nil {
		return err
	}
	pgPort, err := postgresPort(ctx, a, names)
	if err != nil {
		return err
	}

	env := buildTestEnv(password, pgPort, endpoint, s3)
	return runGoTest(ctx, a, args, env)
}

// cleanupContainers removes both throwaway containers with a fresh, timed
// context so a caller cancellation does not also cancel the cleanup itself.
// Its output is discarded and its error ignored, matching tools/test-go.sh.
func cleanupContainers(ctx context.Context, a *app, names containerNames) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	_, _, _ = a.run.run(cleanupCtx, command{
		name: "docker",
		args: []string{"rm", "-f", names.pg, names.s3},
		dir:  a.root,
	})
}

func startPostgres(ctx context.Context, a *app, names containerNames, password string) error {
	_, code, err := a.run.run(ctx, command{
		name: "docker",
		args: []string{
			"run", "-d", "--rm", "--name", names.pg,
			"-p", "127.0.0.1::5432",
			"-e", "POSTGRES_PASSWORD=" + password,
			"-e", "POSTGRES_DB=justixauto_test",
			testPGImage,
		},
		dir: a.root,
	})
	return stepErr(ctx, "start PostgreSQL container", code, err)
}

// startMinio starts the MinIO container when s3 is requested and returns its
// http://127.0.0.1:<port> endpoint; it does nothing and returns "" when s3 is
// false (DT-06: NO_S3=1 adds nothing S3-related).
func startMinio(ctx context.Context, a *app, names containerNames, password string, s3 bool) (string, error) {
	if !s3 {
		return "", nil
	}
	_, code, err := a.run.run(ctx, command{
		name: "docker",
		args: []string{
			"run", "-d", "--rm", "--name", names.s3,
			"-p", "127.0.0.1::9000",
			"-e", "MINIO_ROOT_USER=justixtest",
			"-e", "MINIO_ROOT_PASSWORD=" + password,
			testMinioImage, "server", "/data",
		},
		dir: a.root,
	})
	if e := stepErr(ctx, "start MinIO container", code, err); e != nil {
		return "", e
	}

	out, code, err := a.run.run(ctx, command{
		name: "docker",
		args: []string{"port", names.s3, "9000/tcp"},
		dir:  a.root,
	})
	if e := stepErr(ctx, "get MinIO container port", code, err); e != nil {
		return "", e
	}
	port, perr := parseContainerPort(out)
	if perr != nil {
		return "", fmt.Errorf("get MinIO container port: %w", perr)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

func waitPostgresReady(ctx context.Context, a *app, cfg testConfig, names containerNames) error {
	for i := 0; i < cfg.maxAttempts; i++ {
		_, code, err := a.run.run(ctx, command{
			name: "docker",
			args: []string{"exec", names.pg, "pg_isready", "-U", "postgres", "-d", "justixauto_test", "-h", "127.0.0.1"},
			dir:  a.root,
		})
		if err != nil && ctx.Err() != nil {
			return err
		}
		if err == nil && code == 0 {
			return nil
		}
		if err := sleepCtx(ctx, cfg.attemptDelay); err != nil {
			return err
		}
	}
	return errors.New("PostgreSQL test container not ready after 60s")
}

func waitMinioReady(ctx context.Context, cfg testConfig, endpoint string, s3 bool) error {
	if !s3 {
		return nil
	}
	url := endpoint + "/minio/health/ready"
	for i := 0; i < cfg.maxAttempts; i++ {
		ok, err := cfg.probe(ctx, url)
		if err != nil && ctx.Err() != nil {
			return err
		}
		if ok {
			return nil
		}
		if err := sleepCtx(ctx, cfg.attemptDelay); err != nil {
			return err
		}
	}
	return errors.New("MinIO test container not ready after 60s")
}

func postgresPort(ctx context.Context, a *app, names containerNames) (int, error) {
	out, code, err := a.run.run(ctx, command{
		name: "docker",
		args: []string{"port", names.pg, "5432/tcp"},
		dir:  a.root,
	})
	if e := stepErr(ctx, "get PostgreSQL container port", code, err); e != nil {
		return 0, e
	}
	port, perr := parseContainerPort(out)
	if perr != nil {
		return 0, fmt.Errorf("get PostgreSQL container port: %w", perr)
	}
	return port, nil
}

// buildTestEnv appends the test-only variables after os.Environ() so they
// win over any inherited value (DT-06). With s3 false, nothing S3-related is
// added (NO_S3=1 leaves them absent, not empty).
func buildTestEnv(password string, pgPort int, endpoint string, s3 bool) []string {
	env := append(os.Environ(), fmt.Sprintf(
		"TEST_DATABASE_URL=postgres://postgres:%s@127.0.0.1:%d/justixauto_test?sslmode=disable",
		password, pgPort,
	))
	if s3 {
		env = append(env,
			"TEST_S3_ENDPOINT="+endpoint,
			"AWS_ACCESS_KEY_ID=justixtest",
			"AWS_SECRET_ACCESS_KEY="+password,
			"AWS_REGION=us-east-1",
		)
	}
	return env
}

// runGoTest runs `bash <root>/tools/go.sh test -race -count=1 -p 1 <args>`,
// streaming its output, and maps a non-zero exit to *exitError so realMain
// propagates it unchanged.
func runGoTest(ctx context.Context, a *app, args, env []string) error {
	testArgs := args
	if len(testArgs) == 0 {
		testArgs = []string{"./..."}
	}
	goSh := filepath.Join(a.root, "tools", "go.sh")
	// -p 1: packages share one database.
	runArgs := append([]string{goSh, "test", "-race", "-count=1", "-p", "1"}, testArgs...)

	_, code, err := a.run.run(ctx, command{
		name:   "bash",
		args:   runArgs,
		dir:    a.root,
		env:    env,
		stdout: a.stdout,
		stderr: a.stderr,
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}

// stepErr names a step in an error, e.g. "start PostgreSQL container: exit
// status 125" (never the argv or environment, so the password cannot leak).
// When ctx was cancelled, the raw cancellation cause is returned unwrapped so
// realMain can map it to the signal exit code (130/143).
func stepErr(ctx context.Context, step string, code int, err error) error {
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return fmt.Errorf("%s: %w", step, err)
	}
	if code != 0 {
		return fmt.Errorf("%s: exit status %d", step, code)
	}
	return nil
}

// sleepCtx waits d, or returns the cancellation cause if ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

var errNoContainerPort = errors.New("no usable port in docker port output")

// parseContainerPort reads the first non-empty line of `docker port` output
// and takes the text after its last ':' as a decimal port 1-65535 (handles
// both "127.0.0.1:49153" and "[::]:49153" forms).
func parseContainerPort(out string) (int, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			return 0, errNoContainerPort
		}
		n, err := strconv.Atoi(line[idx+1:])
		if err != nil || n < 1 || n > 65535 {
			return 0, errNoContainerPort
		}
		return n, nil
	}
	return 0, errNoContainerPort
}
