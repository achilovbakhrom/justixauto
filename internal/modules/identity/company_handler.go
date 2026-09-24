package identity

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// CompanyHandler serves company and branch routes for signed-in users.
type CompanyHandler struct {
	companies *CompanyService
	branches  *BranchService
}

type createCompanyRequest struct {
	Company CompanyInput `json:"company"`
}

func (h *CompanyHandler) Routes(g *echo.Group) {
	g.GET("/directory/companies", h.directory, auth.Require())
	g.POST("/companies", h.create, auth.Require(PermCompanyCreate))
	g.GET("/companies/:id", h.get, auth.Require())
	g.PATCH("/companies/:id", h.update, auth.Require())
	g.GET("/companies/:id/branches", h.listBranches, auth.Require())
	g.POST("/companies/:id/branches", h.createBranch, auth.Require())
	g.PATCH("/companies/:id/branches/:branchId", h.updateBranch, auth.Require())
}

// create registers a seller company.
//
//	@Summary	Create seller company
//	@Tags		identity/companies
//	@Security	CSRF
//	@Param		body			body		createCompanyRequest	true	"company"
//	@Success	201				{object}	httpx.DataEnvelope[identity.companyDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/companies [post]
func (h *CompanyHandler) create(c echo.Context) error {
	var in createCompanyRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	company, err := h.companies.CreateSeller(c.Request().Context(), auth.Get(c), in.Company)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toCompany(company), company.Version)
}

// get returns one company.
//
//	@Summary	Get company
//	@Tags		identity/companies
//	@Param		id			path		string	true	"company ID"
//	@Success	200			{object}	httpx.DataEnvelope[identity.companyDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/identity/companies/{id} [get]
func (h *CompanyHandler) get(c echo.Context) error {
	company, err := h.companies.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCompany(company), company.Version)
}

// update edits a company profile.
//
//	@Summary	Update company
//	@Tags		identity/companies
//	@Security	CSRF
//	@Param		id						path		string			true	"company ID"
//	@Param		If-Match				header		string			true	"revision"
//	@Param		body					body		CompanyInput	true	"company"
//	@Success	200						{object}	httpx.DataEnvelope[identity.companyDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/companies/{id} [patch]
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

// listBranches lists a company's branches.
//
//	@Summary	List branches
//	@Tags		identity/companies
//	@Param		id			path		string	true	"company ID"
//	@Success	200			{object}	httpx.ListEnvelope[identity.branchDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/identity/companies/{id}/branches [get]
func (h *CompanyHandler) listBranches(c echo.Context) error {
	branches, err := h.branches.List(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(branches, toBranch), nil)
}

// createBranch adds a branch to a company.
//
//	@Summary	Create branch
//	@Tags		identity/companies
//	@Security	CSRF
//	@Param		id					path		string		true	"company ID"
//	@Param		body				body		BranchInput	true	"branch"
//	@Success	201					{object}	httpx.DataEnvelope[identity.branchDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/companies/{id}/branches [post]
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

// updateBranch edits a branch.
//
//	@Summary	Update branch
//	@Tags		identity/companies
//	@Security	CSRF
//	@Param		id						path		string		true	"company ID"
//	@Param		branchId				path		string		true	"branch ID"
//	@Param		If-Match				header		string		true	"revision"
//	@Param		body					body		BranchInput	true	"branch"
//	@Success	200						{object}	httpx.DataEnvelope[identity.branchDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/companies/{id}/branches/{branchId} [patch]
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

// directory searches active companies.
//
//	@Summary	Company directory
//	@Tags		identity/companies
//	@Param		q		query		string	false	"name search"
//	@Param		kind	query		string	false	"company kind"
//	@Param		limit	query		int		false	"page size"
//	@Param		offset	query		int		false	"offset"
//	@Success	200		{object}	httpx.ListEnvelope[identity.Profile]
//	@Failure	401,422	{object}	httpx.ErrorBody
//	@Router		/identity/directory/companies [get]
func (h *CompanyHandler) directory(c echo.Context) error {
	f := CompanyFilter{Query: c.QueryParam("q"), Kind: CompanyKind(c.QueryParam("kind"))}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	profiles, err := h.companies.Directory(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.List(c, profiles, nil)
}
