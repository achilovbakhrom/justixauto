package telemetry

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// HTTP traces every request (a span per route; otelecho also records
// http.server.request.duration) and logs it. Health probes are skipped.
func HTTP(log *slog.Logger) []echo.MiddlewareFunc {
	skip := func(c echo.Context) bool { p := c.Request().URL.Path; return p == "/healthz" || p == "/readyz" }
	logged := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skip(c) {
				return next(c)
			}
			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err) // write the error response now so the status below is final
			}
			req, res := c.Request(), c.Response()
			route := c.Path()
			elapsed := time.Since(start)
			level := slog.LevelInfo
			if res.Status >= 500 {
				level = slog.LevelError
			}
			log.Log(req.Context(), level, "request", "method", req.Method, "route", route, "path", req.URL.Path, "status", res.Status,
				"duration_ms", elapsed.Milliseconds(), "request_id", res.Header().Get(echo.HeaderXRequestID), "bytes", res.Size)
			return nil
		}
	}
	return []echo.MiddlewareFunc{otelecho.Middleware(ServiceName, otelecho.WithSkipper(skip)), logged}
}
