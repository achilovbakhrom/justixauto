package telemetry

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HTTP traces every request (a span per route) and then logs it and records
// http.server.request.duration. Health probes are not traced or logged.
func HTTP(log *slog.Logger) []echo.MiddlewareFunc {
	skip := func(c echo.Context) bool { p := c.Request().URL.Path; return p == "/healthz" || p == "/readyz" }
	duration, _ := otel.Meter("justixauto/http").Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"), metric.WithDescription("Duration of HTTP server requests"))
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
			duration.Record(req.Context(), elapsed.Seconds(), metric.WithAttributes(attribute.String("http.request.method", req.Method),
				attribute.String("http.route", route), attribute.Int("http.response.status_code", res.Status)))
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
