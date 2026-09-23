package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestDrainClosesConnections(t *testing.T) {
	var d Drain
	e := echo.New()
	e.Use(d.Middleware)
	e.GET("/", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	get := func() string {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return rec.Header().Get("Connection")
	}
	if get() != "" {
		t.Fatal("keep-alive must stay before draining")
	}
	d.Start()
	if get() != "close" {
		t.Fatal("draining must close connections")
	}
}
