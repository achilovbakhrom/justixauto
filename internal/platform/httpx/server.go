// Package httpx configures the Echo server and maps service errors to HTTP.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	e.GET("/healthz", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	return e
}

type errorBody struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields,omitempty"`
}

func errorHandler(log *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status, body := http.StatusInternalServerError, errorBody{Error: "internal error"}
		var validation *apperr.ValidationError
		var httpErr *echo.HTTPError
		switch {
		case errors.As(err, &validation):
			status, body = http.StatusUnprocessableEntity, errorBody{Error: "validation failed", Fields: validation.Fields}
		case errors.Is(err, apperr.ErrNotFound):
			status, body = http.StatusNotFound, errorBody{Error: "not found"}
		case errors.Is(err, apperr.ErrConflict):
			status, body = http.StatusConflict, errorBody{Error: err.Error()}
		case errors.Is(err, apperr.ErrStale):
			status, body = http.StatusPreconditionFailed, errorBody{Error: "stale version, reload and retry"}
		case errors.As(err, &httpErr):
			status, body = httpErr.Code, errorBody{Error: http.StatusText(httpErr.Code)}
		default:
			log.Error("request failed", "method", c.Request().Method, "path", c.Path(), "err", err)
		}
		if err := c.JSON(status, body); err != nil {
			log.Error("write error response", "err", err)
		}
	}
}

// IfMatchVersion parses a required If-Match header of the form "N" into a version.
func IfMatchVersion(c echo.Context) (int64, error) {
	raw := strings.Trim(c.Request().Header.Get("If-Match"), `"`)
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 1 {
		return 0, echo.NewHTTPError(http.StatusPreconditionRequired)
	}
	return v, nil
}

// SetETag exposes an entity version so clients can send it back via If-Match.
func SetETag(c echo.Context, version int64) {
	c.Response().Header().Set("ETag", `"`+strconv.FormatInt(version, 10)+`"`)
}
