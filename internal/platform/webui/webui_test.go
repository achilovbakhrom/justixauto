package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMount(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "realization", "dist", "index.html"), "seller")
	write(t, filepath.Join(root, "realization", "dist", "assets", "app-1.js"), "js")
	write(t, filepath.Join(root, "financing", "dist", "index.html"), "finance")
	e := echo.New()
	e.GET("/api/v1/ping", func(c echo.Context) error { return c.String(http.StatusOK, "pong") })
	Mount(e, Apps(root))

	cases := []struct {
		path, body, cache string
		status            int
	}{
		{"/", "seller", "no-cache", 200},
		{"/sales/123", "seller", "no-cache", 200},
		{"/assets/app-1.js", "js", "public, max-age=31536000, immutable", 200},
		{"/assets/missing.js", "", "", 404},
		{"/finance/applications", "finance", "no-cache", 200},
		{"/finance", "", "", 302},
		{"/insurance/x", "seller", "no-cache", 200}, // app not built: the root app answers
		{"/api/v1/ping", "pong", "", 200},
		{"/api/v1/unknown", "", "", 404},
		{"/../../etc/passwd", "seller", "no-cache", 200},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Fatalf("%s: status %d, want %d", tc.path, rec.Code, tc.status)
		}
		if tc.body != "" && rec.Body.String() != tc.body {
			t.Fatalf("%s: body %q, want %q", tc.path, rec.Body.String(), tc.body)
		}
		if tc.cache != "" && rec.Header().Get("Cache-Control") != tc.cache {
			t.Fatalf("%s: cache %q", tc.path, rec.Header().Get("Cache-Control"))
		}
	}
}
