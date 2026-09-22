package identity

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

// CompanyHandler serves company and branch routes for signed-in users.
type CompanyHandler struct {
	companies *CompanyService
	branches  *BranchService
}

func (h *CompanyHandler) Routes(g *echo.Group) {
	g.POST("/companies", h.create, auth.Require(PermCompanyCreate))
	g.GET("/companies/:id", h.get, auth.Require())
	g.PATCH("/companies/:id", h.update, auth.Require())
	g.GET("/companies/:id/branches", h.listBranches, auth.Require())
	g.POST("/companies/:id/branches", h.createBranch, auth.Require())
	g.PATCH("/companies/:id/branches/:branchId", h.updateBranch, auth.Require())
}

func (h *CompanyHandler) create(c echo.Context) error {
	var in struct {
		Company CompanyInput `json:"company"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	company, err := h.companies.CreateSeller(c.Request().Context(), auth.Get(c), in.Company)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toCompany(company), company.Version)
}

func (h *CompanyHandler) get(c echo.Context) error {
	company, err := h.companies.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCompany(company), company.Version)
}

func (h *CompanyHandler) update(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in CompanyInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	company, err := h.companies.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCompany(company), company.Version)
}

func (h *CompanyHandler) listBranches(c echo.Context) error {
	branches, err := h.branches.List(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(branches, toBranch), nil)
}

func (h *CompanyHandler) createBranch(c echo.Context) error {
	var in BranchInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	b, err := h.branches.Create(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toBranch(b), b.Version)
}

func (h *CompanyHandler) updateBranch(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in BranchInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	b, err := h.branches.Update(c.Request().Context(), auth.Get(c), c.Param("id"), c.Param("branchId"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toBranch(b), b.Version)
}
