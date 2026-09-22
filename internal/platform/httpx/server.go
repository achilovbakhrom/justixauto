// Package httpx configures the Echo server, the response envelopes of the HTTP
// contract (docs/justix-auto/contracts/http-domain.md) and error mapping.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"justixauto/internal/platform/apperr"
)

// NewServer returns an Echo instance with shared middleware and error handling.
func NewServer(log *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler(log)
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.BodyLimit("1M"))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// API responses are per-user and must never be cached.
			c.Response().Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	})
	e.GET("/healthz", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	return e
}

// Revision renders a version number as the contract's decimal-string revision.
func Revision(version int64) string { return strconv.FormatInt(version, 10) }

type dataEnvelope struct {
	Data     any    `json:"data"`
	Revision string `json:"revision"`
	AsOf     string `json:"asOf,omitempty"`
}

// Data writes {data, revision} and an ETag carrying the same revision.
func Data(c echo.Context, status int, data any, version int64) error {
	c.Response().Header().Set("ETag", `"`+Revision(version)+`"`)
	body := dataEnvelope{Data: data, Revision: Revision(version)}
	if c.Request().Method == http.MethodGet {
		body.AsOf = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return c.JSON(status, body)
}

// List writes {items, nextCursor, asOf}. Items must be a non-nil slice.
func List(c echo.Context, items any, nextCursor *string) error {
	return c.JSON(http.StatusOK, map[string]any{
		"items":      items,
		"nextCursor": nextCursor,
		"asOf":       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// IfMatch parses the required If-Match header ("N") into a version.
func IfMatch(c echo.Context) (int64, error) {
	raw := c.Request().Header.Get("If-Match")
	if raw == "" {
		return 0, apperr.New(apperr.ErrPreconditionRequired, "if_match_required", "If-Match header with the current revision is required")
	}
	v, err := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
	if err != nil || v < 0 {
		return 0, apperr.FieldError("If-Match", "must be a revision number")
	}
	return v, nil
}

// Bind decodes the JSON body, turning decode failures into 400.
func Bind(c echo.Context, dst any) error {
	if err := c.Bind(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	return nil
}

// IntQuery parses an optional integer query parameter.
func IntQuery(c echo.Context, name string) (int, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperr.FieldError(name, "must be a number")
	}
	return n, nil
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields"`
	TraceID string            `json:"traceId"`
}

var kinds = []struct {
	kind          error
	status        int
	code, message string
}{
	{apperr.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{apperr.ErrConflict, http.StatusConflict, "conflict", "conflict"},
	{apperr.ErrStale, http.StatusPreconditionFailed, "stale_revision", "the resource has changed, reload and retry"},
	{apperr.ErrPreconditionRequired, http.StatusPreconditionRequired, "precondition_required", "precondition required"},
	{apperr.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "sign in required"},
	{apperr.ErrForbidden, http.StatusForbidden, "forbidden", "not allowed"},
}

func errorHandler(log *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status := http.StatusInternalServerError
		detail := errorDetail{Code: "internal", Message: "internal error", Fields: map[string]string{}}
		var validation *apperr.ValidationError
		var coded *apperr.Error
		var limited *apperr.RateLimitedError
		var httpErr *echo.HTTPError
		switch {
		case errors.As(err, &validation):
			status, detail.Code, detail.Message, detail.Fields = http.StatusUnprocessableEntity, "validation_failed", "validation failed", validation.Fields
		case errors.As(err, &limited):
			status, detail.Code, detail.Message = http.StatusTooManyRequests, "rate_limited", "too many attempts, retry later"
			c.Response().Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())+1))
		case errors.As(err, &httpErr):
			status, detail.Code, detail.Message = httpErr.Code, strings.ToLower(strings.ReplaceAll(http.StatusText(httpErr.Code), " ", "_")), http.StatusText(httpErr.Code)
		default:
			matched := false
			for _, k := range kinds {
				if errors.Is(err, k.kind) {
					status, detail.Code, detail.Message, matched = k.status, k.code, k.message, true
					break
				}
			}
			if matched && errors.As(err, &coded) {
				detail.Code, detail.Message = coded.Code, coded.Message
			}
			if !matched {
				log.Error("request failed", "method", c.Request().Method, "path", c.Path(), "err", err)
			}
		}
		detail.TraceID = c.Response().Header().Get(echo.HeaderXRequestID)
		if err := c.JSON(status, errorBody{Error: detail}); err != nil {
			log.Error("write error response", "err", err)
		}
	}
}
