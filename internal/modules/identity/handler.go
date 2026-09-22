package identity

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/httpx"
)

// CompanyHandler translates HTTP to CompanyService calls. No business rules here.
type CompanyHandler struct{ service *CompanyService }

func NewCompanyHandler(service *CompanyService) *CompanyHandler {
	return &CompanyHandler{service: service}
}

func (h *CompanyHandler) Routes(g *echo.Group) {
	g.POST("/companies", h.create)
	g.GET("/companies", h.list)
	g.GET("/companies/:id", h.get)
	g.PUT("/companies/:id", h.update)
	g.POST("/companies/:id/status", h.changeStatus)
}

func (h *CompanyHandler) create(c echo.Context) error {
	var in CreateCompanyInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	company, err := h.service.Create(c.Request().Context(), in)
	if err != nil {
		return err
	}
	httpx.SetETag(c, company.Version)
	return c.JSON(http.StatusCreated, company)
}

func (h *CompanyHandler) list(c echo.Context) error {
	f := CompanyFilter{Kind: CompanyKind(c.QueryParam("kind")), Status: CompanyStatus(c.QueryParam("status"))}
	var err error
	if f.Limit, err = intParam(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = intParam(c, "offset"); err != nil {
		return err
	}
	companies, err := h.service.List(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"items": companies})
}

func (h *CompanyHandler) get(c echo.Context) error {
	company, err := h.service.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	httpx.SetETag(c, company.Version)
	return c.JSON(http.StatusOK, company)
}

func (h *CompanyHandler) update(c echo.Context) error {
	version, err := httpx.IfMatchVersion(c)
	if err != nil {
		return err
	}
	var in UpdateCompanyInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	company, err := h.service.Update(c.Request().Context(), c.Param("id"), version, in)
	if err != nil {
		return err
	}
	httpx.SetETag(c, company.Version)
	return c.JSON(http.StatusOK, company)
}

func (h *CompanyHandler) changeStatus(c echo.Context) error {
	version, err := httpx.IfMatchVersion(c)
	if err != nil {
		return err
	}
	var in ChangeStatusInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	company, err := h.service.ChangeStatus(c.Request().Context(), c.Param("id"), version, in)
	if err != nil {
		return err
	}
	httpx.SetETag(c, company.Version)
	return c.JSON(http.StatusOK, company)
}

func intParam(c echo.Context, name string) (int, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest)
	}
	return n, nil
}
