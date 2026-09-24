package httpx

import (
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// Drain tells clients to reconnect elsewhere while a replica shuts down: after
// Start, every response carries "Connection: close", so keep-alive connections
// move to other pods before this one stops listening.
type Drain struct{ on atomic.Bool }

func (d *Drain) Start() { d.on.Store(true) }

func (d *Drain) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if d.on.Load() {
			c.Response().Header().Set(echo.HeaderConnection, "close")
		}
		return next(c)
	}
}
