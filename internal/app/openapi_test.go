package app_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"justixauto/internal/app"
	"justixauto/internal/e2e"
	"justixauto/internal/modules/identity"
	"justixauto/internal/pkg/apidocs"
)

// Every /api/v1 route must be annotated for the generated spec (make openapi),
// and the spec must not list routes that no longer exist.
func TestOpenAPICoversRoutes(t *testing.T) {
	e, _, err := app.New(e2e.DB(t), app.Config{
		Session: identity.DefaultSessionConfig,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	param := regexp.MustCompile(`:(\w+)`)
	var routes []string
	for _, r := range e.Routes() {
		path, ok := strings.CutPrefix(r.Path, "/api/v1/")
		if !ok || strings.HasSuffix(path, "*") || r.Method == echo.RouteNotFound {
			continue // health probes, web apps, group fallbacks
		}
		routes = append(routes, r.Method+" /"+param.ReplaceAllString(path, "{$1}"))
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []struct{ Name, In string } `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(apidocs.Spec(), &spec); err != nil {
		t.Fatal(err)
	}
	var documented []string
	for path, ops := range spec.Paths {
		for method, op := range ops {
			documented = append(documented, strings.ToUpper(method)+" "+path)
			// Writes are idempotent by their data (unique keys, state checks,
			// If-Match), never by a request key.
			if slices.ContainsFunc(op.Parameters, func(p struct{ Name, In string }) bool {
				return p.In == "header" && strings.EqualFold(p.Name, "Idempotency-Key")
			}) {
				t.Errorf("%s %s documents an Idempotency-Key header", strings.ToUpper(method), path)
			}
		}
	}
	for _, r := range routes {
		if !slices.Contains(documented, r) {
			t.Errorf("route %s has no @Router annotation (then run make openapi)", r)
		}
	}
	for _, d := range documented {
		if !slices.Contains(routes, d) {
			t.Errorf("spec documents %s, which is not a route", d)
		}
	}
}
