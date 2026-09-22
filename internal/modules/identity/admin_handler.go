package identity

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

// AdminHandler serves the Admin app: registry, users, roles, memberships, audit.
type AdminHandler struct {
	companies   *CompanyService
	users       *UserService
	roles       *RoleService
	memberships *MembershipService
	audit       *AuditService
}

func (h *AdminHandler) Routes(g *echo.Group) {
	a := g.Group("/admin")
	a.POST("/provider-companies", h.createProvider, auth.Require(PermPlatformCompaniesCreate))
	a.GET("/companies", h.listCompanies, auth.Require(PermPlatformDirectoryRead))
	a.POST("/companies/:id/:action", h.companyAccess, auth.Require(PermPlatformCompaniesAccess))

	a.GET("/users", h.listUsers, auth.Require(PermPlatformUsersManage))
	a.POST("/users", h.createUser, auth.Require(PermPlatformUsersManage))
	a.GET("/users/:id", h.getUser, auth.Require(PermPlatformUsersManage))
	a.PATCH("/users/:id", h.updateUser, auth.Require(PermPlatformUsersManage))
	a.POST("/users/:id/suspend", h.userStatus(true), auth.Require(PermPlatformUsersManage))
	a.POST("/users/:id/restore", h.userStatus(false), auth.Require(PermPlatformUsersManage))
	a.POST("/users/:id/revoke-sessions", h.revokeSessions, auth.Require(PermPlatformUsersManage))

	a.GET("/users/:id/memberships", h.listMemberships, auth.Require(PermPlatformMembershipsManage))
	a.POST("/users/:id/memberships", h.grantMembership, auth.Require(PermPlatformMembershipsManage))
	a.PATCH("/memberships/:id/branch-access", h.membershipAccess, auth.Require(PermPlatformMembershipsManage))
	a.POST("/memberships/:id/revoke", h.revokeMembership, auth.Require(PermPlatformMembershipsManage))

	a.GET("/permissions", h.listPermissions, auth.Require(PermPlatformRolesManage))
	a.GET("/roles", h.listRoles, auth.Require(PermPlatformRolesManage))
	a.POST("/roles", h.createRole, auth.Require(PermPlatformRolesManage))
	a.PATCH("/roles/:id", h.updateRole, auth.Require(PermPlatformRolesManage))

	a.GET("/audit", h.listAudit, auth.Require(PermPlatformAuditRead))
}

type reasonBody struct {
	Reason string `json:"reason"`
}

func (h *AdminHandler) createProvider(c echo.Context) error {
	var in ProviderInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.companies.CreateProvider(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	data := map[string]any{
		"company":    toCompany(r.Company),
		"admin":      map[string]any{"id": r.Admin.ID, "login": r.Admin.Login, "displayName": r.Admin.DisplayName},
		"membership": toMembership(r.Membership),
	}
	return httpx.Data(c, http.StatusCreated, data, r.Company.Version)
}

func (h *AdminHandler) listCompanies(c echo.Context) error {
	f := CompanyFilter{Kind: CompanyKind(c.QueryParam("kind")), Access: CompanyAccess(c.QueryParam("access"))}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	companies, err := h.companies.List(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(companies, toCompany), nil)
}

func (h *AdminHandler) companyAccess(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in reasonBody
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	company, err := h.companies.SetAccess(c.Request().Context(), auth.Get(c), c.Param("id"), expected, c.Param("action"), in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCompany(company), company.Version)
}

func (h *AdminHandler) listUsers(c echo.Context) error {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := httpx.IntQuery(c, "offset")
	if err != nil {
		return err
	}
	users, err := h.users.List(c.Request().Context(), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(users, toUser), nil)
}

func (h *AdminHandler) createUser(c echo.Context) error {
	var in CreateUserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.users.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toUser(u), u.User.Version)
}

func (h *AdminHandler) getUser(c echo.Context) error {
	u, err := h.users.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
}

func (h *AdminHandler) updateUser(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in UpdateUserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.users.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
}

func (h *AdminHandler) userStatus(suspend bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in reasonBody
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
		change := h.users.Restore
		if suspend {
			change = h.users.Suspend
		}
		u, err := change(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
		if err != nil {
			return err
		}
		return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
	}
}

func (h *AdminHandler) revokeSessions(c echo.Context) error {
	var in reasonBody
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := h.users.RevokeSessions(c.Request().Context(), auth.Get(c), c.Param("id"), in.Reason); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AdminHandler) listMemberships(c echo.Context) error {
	ms, err := h.memberships.ListByUser(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ms, toMembership), nil)
}

func (h *AdminHandler) grantMembership(c echo.Context) error {
	var in GrantMembershipInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.memberships.Grant(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toMembership(m), m.Version)
}

func (h *AdminHandler) membershipAccess(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		BranchAccess BranchAccessInput `json:"branchAccess"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.memberships.UpdateBranchAccess(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.BranchAccess)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toMembership(m), m.Version)
}

func (h *AdminHandler) revokeMembership(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in reasonBody
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.memberships.Revoke(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toMembership(m), m.Version)
}

func (h *AdminHandler) listPermissions(c echo.Context) error {
	return httpx.List(c, Catalog, nil)
}

func (h *AdminHandler) listRoles(c echo.Context) error {
	roles, err := h.roles.List(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(roles, toRole), nil)
}

func (h *AdminHandler) createRole(c echo.Context) error {
	var in RoleInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.roles.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toRole(r), r.Version)
}

func (h *AdminHandler) updateRole(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in RoleInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.roles.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toRole(r), r.Version)
}

func (h *AdminHandler) listAudit(c echo.Context) error {
	f := AuditFilter{ResourceType: c.QueryParam("resourceType"), ResourceID: c.QueryParam("resourceId"), ActorID: c.QueryParam("actorId")}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	events, err := h.audit.List(c.Request().Context(), f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(events, toAudit), nil)
}
