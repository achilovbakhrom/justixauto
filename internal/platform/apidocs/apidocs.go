// Package apidocs serves Swagger UI and the OpenAPI spec generated from the
// handler annotations (make openapi → swagger.json in this directory).
package apidocs

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v4"
	swaggerfiles "github.com/swaggo/files/v2"
)

//go:embed swagger.json
var spec []byte

// Spec returns the generated OpenAPI (Swagger 2.0) document.
func Spec() []byte { return spec }

// The UI loads the spec next to it; the stock initializer points at the petstore.
const initializer = `window.onload = function () {
  window.ui = SwaggerUIBundle({
    url: "./swagger.json",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    layout: "StandaloneLayout",
  });
};
`

// Mount serves the UI at /api/docs/ and the spec at /api/docs/swagger.json.
func Mount(e *echo.Echo) {
	assets := http.StripPrefix("/api/docs/", http.FileServer(http.FS(swaggerfiles.FS)))
	e.GET("/api/docs", func(c echo.Context) error { return c.Redirect(http.StatusMovedPermanently, "/api/docs/") })
	e.GET("/api/docs/swagger.json", func(c echo.Context) error { return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, spec) })
	e.GET("/api/docs/swagger-initializer.js", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "text/javascript; charset=utf-8", []byte(initializer))
	})
	e.GET("/api/docs/*", echo.WrapHandler(assets))
}
