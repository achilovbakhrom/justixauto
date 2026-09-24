// Package handler holds commerce's Echo routes, request/response DTOs and
// mappers. It imports service, model and internal/pkg only; it must never
// import repository or gorm.
package handler

import (
	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/commerce/service"
	"justixauto/internal/pkg/httpx"
)

type counterpartyDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

func party(c service.Company) counterpartyDTO {
	return counterpartyDTO{ID: c.ID, Name: c.Name, Country: c.Country}
}

func mapSlice[T, D any](items []T, f func(*T) D) []D {
	out := make([]D, len(items))
	for i := range items {
		out[i] = f(&items[i])
	}
	return out
}

func paging(c echo.Context) (int, int, error) {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return 0, 0, err
	}
	offset, err := httpx.IntQuery(c, "offset")
	return limit, offset, err
}
