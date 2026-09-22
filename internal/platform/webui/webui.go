// Package webui serves the built React apps next to the API, so one binary
// can run the whole product. Each app owns a URL prefix; unknown paths inside
// a prefix get the app's index.html (client-side routing).
package webui

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

// App is one built single-page app.
type App struct {
	Prefix string // "/finance/"; "/" for the root app
	Dir    string // directory with index.html and assets/
}

// Apps returns the four JustixAuto apps under a web/apps checkout.
func Apps(root string) []App {
	return []App{
		{Prefix: "/finance/", Dir: filepath.Join(root, "financing", "dist")},
		{Prefix: "/insurance/", Dir: filepath.Join(root, "insurance", "dist")},
		{Prefix: "/admin/", Dir: filepath.Join(root, "admin", "dist")},
		{Prefix: "/", Dir: filepath.Join(root, "realization", "dist")},
	}
}

// Mount serves the apps for GET/HEAD requests outside /api/ and /healthz.
// Apps whose directory has no index.html are skipped.
func Mount(e *echo.Echo, apps []App) {
	var ready []App
	for _, a := range apps {
		if _, err := os.Stat(filepath.Join(a.Dir, "index.html")); err == nil {
			ready = append(ready, a)
		}
	}
	if len(ready) == 0 {
		return
	}
	h := func(c echo.Context) error {
		p := c.Request().URL.Path
		if strings.HasPrefix(p, "/api/") {
			return echo.ErrNotFound
		}
		for _, a := range ready {
			if a.Prefix != "/" && p == strings.TrimSuffix(a.Prefix, "/") {
				return c.Redirect(http.StatusFound, a.Prefix)
			}
			if strings.HasPrefix(p, a.Prefix) {
				return serve(c, a, strings.TrimPrefix(p, a.Prefix))
			}
		}
		return echo.ErrNotFound
	}
	e.GET("/*", h)
	e.HEAD("/*", h)
}

func serve(c echo.Context, a App, rel string) error {
	hdr := c.Response().Header()
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("X-Frame-Options", "DENY")
	hdr.Set("Referrer-Policy", "same-origin")
	// Styles are injected by the kit at runtime, so inline styles are allowed; scripts are not.
	hdr.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; "+
		"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
	clean := path.Clean("/" + rel)
	if clean != "/" && !strings.HasSuffix(clean, ".html") {
		file := filepath.Join(a.Dir, filepath.FromSlash(clean))
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			if strings.HasPrefix(clean, "/assets/") {
				// Vite puts a content hash in every asset name.
				hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				hdr.Set("Cache-Control", "no-cache")
			}
			return c.File(file)
		}
		if path.Ext(clean) != "" {
			return echo.ErrNotFound // a missing asset is not a page
		}
	}
	hdr.Set("Cache-Control", "no-cache")
	return c.File(filepath.Join(a.Dir, "index.html"))
}
