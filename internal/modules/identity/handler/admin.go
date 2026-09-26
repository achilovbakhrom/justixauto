package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/modules/identity/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// AdminHandler serves the Admin app: registry, users, roles, memberships, audit.
type AdminHandler struct {
	companies   *service.Company
	users       *service.User
	roles       *service.Role
	memberships *service.Membership
	audit       *service.Audit
}

// NewAdminHandler builds the admin handler.
func NewAdminHandler(companies *service.Company, users *service.User, roles *service.Role, memberships *service.Membership, audit *service.Audit) *AdminHandler {
	return &AdminHandler{companies: companies, users: users, roles: roles, memberships: memberships, audit: audit}
}

func (h *AdminHandler) Routes(g *echo.Group) {
	a := g.Group("/admin")
	a.POST("/provider-companies", h.createProvider, auth.Require(model.PermPlatformCompaniesCreate))
	a.POST("/seller-companies", h.createSeller, auth.Require(model.PermPlatformCompaniesCreate))
	a.GET("/companies", h.listCompanies, auth.Require(model.PermPlatformDirectoryRead))
	a.POST("/companies/:id/:action", h.companyAccess, auth.Require(model.PermPlatformCompaniesAccess))

	a.GET("/users", h.listUsers, auth.Require(model.PermPlatformUsersManage))
	a.POST("/users", h.createUser, auth.Require(model.PermPlatformUsersManage))
	a.GET("/users/:id", h.getUser, auth.Require(model.PermPlatformUsersManage))
	a.PATCH("/users/:id", h.updateUser, auth.Require(model.PermPlatformUsersManage))
	a.POST("/users/:id/suspend", h.userStatus(true), auth.Require(model.PermPlatformUsersManage))
	a.POST("/users/:id/restore", h.userStatus(false), auth.Require(model.PermPlatformUsersManage))
	a.POST("/users/:id/revoke-sessions", h.revokeSessions, auth.Require(model.PermPlatformUsersManage))
	a.POST("/users/:id/password", h.setPassword, auth.Require(model.PermPlatformUsersManage))

	a.GET("/users/:id/memberships", h.listMemberships, auth.Require(model.PermPlatformMembershipsManage))
	a.POST("/users/:id/memberships", h.grantMembership, auth.Require(model.PermPlatformMembershipsManage))
	a.PATCH("/memberships/:id/branch-access", h.membershipAccess, auth.Require(model.PermPlatformMembershipsManage))
	a.POST("/memberships/:id/revoke", h.revokeMembership, auth.Require(model.PermPlatformMembershipsManage))

	a.GET("/permissions", h.listPermissions, auth.Require(model.PermPlatformRolesManage))
	a.GET("/roles", h.listRoles, auth.Require(model.PermPlatformRolesManage))
	a.POST("/roles", h.createRole, auth.Require(model.PermPlatformRolesManage))
	a.PATCH("/roles/:id", h.updateRole, auth.Require(model.PermPlatformRolesManage))

	a.GET("/audit", h.listAudit, auth.Require(model.PermPlatformAuditRead))
}

type reasonBody struct {
	Reason string `json:"reason"`
}

// provisionedAdmin is the seed admin user created with a new company.
type provisionedAdmin struct {
	ID          string  `json:"id"`
	Login       *string `json:"login"`
	DisplayName string  `json:"displayName"`
}

// provisionedResponse is the response for provisioning a new company with its admin.
type provisionedResponse struct {
	Company    companyDTO       `json:"company"`
	Admin      provisionedAdmin `json:"admin"`
	Membership membershipDTO    `json:"membership"`
}

type membershipAccessRequest struct {
	BranchAccess service.BranchAccessInput `json:"branchAccess"`
}

// createProvider registers a provider company with its admin user.
//
//	@Summary	Create provider company
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		body			body		service.ProviderInput	true	"provider company"
//	@Success	201				{object}	httpx.DataEnvelope[handler.provisionedResponse]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/provider-companies [post]
func (h *AdminHandler) createProvider(c echo.Context) error {
	var in service.ProviderInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.companies.CreateProvider(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return provisioned(c, r)
}

// createSeller registers a seller company with its admin user.
//
//	@Summary	Create seller company (admin)
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		body			body		service.SellerInput	true	"seller company"
//	@Success	201				{object}	httpx.DataEnvelope[handler.provisionedResponse]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/seller-companies [post]
func (h *AdminHandler) createSeller(c echo.Context) error {
	var in service.SellerInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.companies.CreateSellerWithAdmin(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return provisioned(c, r)
}

// setPassword sets a user's password (admin reset).
//
//	@Summary	Set user password
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string						true	"user ID"
//	@Param		If-Match				header		string						true	"revision"
//	@Param		body					body		service.SetPasswordInput	true	"password"
//	@Success	200						{object}	httpx.DataEnvelope[handler.userDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id}/password [post]
func (h *AdminHandler) setPassword(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.SetPasswordInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.users.SetPassword(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
}

func provisioned(c echo.Context, r *service.ProvisionResult) error {
	data := provisionedResponse{
		Company:    toCompany(r.Company),
		Admin:      provisionedAdmin{ID: r.Admin.ID, Login: r.Admin.Login, DisplayName: r.Admin.DisplayName},
		Membership: toMembership(r.Membership),
	}
	return httpx.Data(c, http.StatusCreated, data, r.Company.Version)
}

// listCompanies lists companies for the platform registry.
//
//	@Summary	List companies (admin)
//	@Tags		identity/admin
//	@Param		kind	query		string	false	"company kind"
//	@Param		access	query		string	false	"access state"
//	@Param		limit	query		int		false	"page size"
//	@Param		offset	query		int		false	"offset"
//	@Success	200		{object}	httpx.ListEnvelope[handler.companyDTO]
//	@Failure	401,403	{object}	httpx.ErrorBody
//	@Router		/identity/admin/companies [get]
func (h *AdminHandler) listCompanies(c echo.Context) error {
	f := model.CompanyFilter{Kind: model.CompanyKind(c.QueryParam("kind")), Access: model.CompanyAccess(c.QueryParam("access"))}
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

// companyAccess changes a company's platform access (activate, suspend,
// restore) or soft-deletes it (delete: hidden everywhere, history kept).
//
//	@Summary	Change company access (admin)
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string		true	"company ID"
//	@Param		action					path		string		true	"activate | suspend | restore | delete"
//	@Param		If-Match				header		string		true	"revision"
//	@Param		body					body		reasonBody	true	"reason"
//	@Success	200						{object}	httpx.DataEnvelope[handler.companyDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/companies/{id}/{action} [post]
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

// listUsers lists platform users.
//
//	@Summary	List users
//	@Tags		identity/admin
//	@Param		limit	query		int	false	"page size"
//	@Param		offset	query		int	false	"offset"
//	@Success	200		{object}	httpx.ListEnvelope[handler.userDTO]
//	@Failure	401,403	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users [get]
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

// createUser creates a platform user account.
//
//	@Summary	Create user
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		body			body		service.CreateUserInput	true	"user"
//	@Success	201				{object}	httpx.DataEnvelope[handler.userDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users [post]
func (h *AdminHandler) createUser(c echo.Context) error {
	var in service.CreateUserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.users.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toUser(u), u.User.Version)
}

// getUser returns one user.
//
//	@Summary	Get user
//	@Tags		identity/admin
//	@Param		id			path		string	true	"user ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.userDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id} [get]
func (h *AdminHandler) getUser(c echo.Context) error {
	u, err := h.users.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
}

// updateUser edits a user's profile.
//
//	@Summary	Update user
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string					true	"user ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		service.UpdateUserInput	true	"user"
//	@Success	200						{object}	httpx.DataEnvelope[handler.userDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id} [patch]
func (h *AdminHandler) updateUser(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.UpdateUserInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := h.users.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toUser(u), u.User.Version)
}

// userStatus suspends or restores a user account.
//
//	@Summary	Suspend or restore user
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string		true	"user ID"
//	@Param		If-Match				header		string		true	"revision"
//	@Param		body					body		reasonBody	true	"reason"
//	@Success	200						{object}	httpx.DataEnvelope[handler.userDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id}/suspend [post]
//	@Router		/identity/admin/users/{id}/restore [post]
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

// revokeSessions revokes every active session for a user.
//
//	@Summary	Revoke user sessions
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id				path	string		true	"user ID"
//	@Param		body			body	reasonBody	true	"reason"
//	@Success	204				"no content"
//	@Failure	401,403,404,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id}/revoke-sessions [post]
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

// listMemberships lists a user's company memberships.
//
//	@Summary	List user memberships
//	@Tags		identity/admin
//	@Param		id			path		string	true	"user ID"
//	@Success	200			{object}	httpx.ListEnvelope[handler.membershipDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id}/memberships [get]
func (h *AdminHandler) listMemberships(c echo.Context) error {
	ms, err := h.memberships.ListByUser(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ms, toMembership), nil)
}

// grantMembership grants a user membership in a company.
//
//	@Summary	Grant membership
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id					path		string							true	"user ID"
//	@Param		body				body		service.GrantMembershipInput	true	"membership"
//	@Success	201					{object}	httpx.DataEnvelope[handler.membershipDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/users/{id}/memberships [post]
func (h *AdminHandler) grantMembership(c echo.Context) error {
	var in service.GrantMembershipInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.memberships.Grant(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toMembership(m), m.Version)
}

// membershipAccess edits a membership's branch access.
//
//	@Summary	Update membership branch access
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string					true	"membership ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		membershipAccessRequest	true	"branch access"
//	@Success	200						{object}	httpx.DataEnvelope[handler.membershipDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/memberships/{id}/branch-access [patch]
func (h *AdminHandler) membershipAccess(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in membershipAccessRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	m, err := h.memberships.UpdateBranchAccess(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.BranchAccess)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toMembership(m), m.Version)
}

// revokeMembership revokes a company membership.
//
//	@Summary	Revoke membership
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string		true	"membership ID"
//	@Param		If-Match				header		string		true	"revision"
//	@Param		body					body		reasonBody	true	"reason"
//	@Success	200						{object}	httpx.DataEnvelope[handler.membershipDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/memberships/{id}/revoke [post]
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

// listPermissions lists the permission catalog.
//
//	@Summary	List permission catalog
//	@Tags		identity/admin
//	@Success	200		{object}	httpx.ListEnvelope[model.PermissionInfo]
//	@Failure	401,403	{object}	httpx.ErrorBody
//	@Router		/identity/admin/permissions [get]
func (h *AdminHandler) listPermissions(c echo.Context) error {
	return httpx.List(c, model.Catalog, nil)
}

// listRoles lists roles.
//
//	@Summary	List roles
//	@Tags		identity/admin
//	@Success	200		{object}	httpx.ListEnvelope[handler.roleDTO]
//	@Failure	401,403	{object}	httpx.ErrorBody
//	@Router		/identity/admin/roles [get]
func (h *AdminHandler) listRoles(c echo.Context) error {
	roles, err := h.roles.List(c.Request().Context())
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(roles, toRole), nil)
}

// createRole creates a role.
//
//	@Summary	Create role
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		body			body		service.RoleInput	true	"role"
//	@Success	201				{object}	httpx.DataEnvelope[handler.roleDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/identity/admin/roles [post]
func (h *AdminHandler) createRole(c echo.Context) error {
	var in service.RoleInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.roles.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toRole(r), r.Version)
}

// updateRole edits a role.
//
//	@Summary	Update role
//	@Tags		identity/admin
//	@Security	CSRF
//	@Param		id						path		string				true	"role ID"
//	@Param		If-Match				header		string				true	"revision"
//	@Param		body					body		service.RoleInput	true	"role"
//	@Success	200						{object}	httpx.DataEnvelope[handler.roleDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/admin/roles/{id} [patch]
func (h *AdminHandler) updateRole(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.RoleInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.roles.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toRole(r), r.Version)
}

// listAudit lists audit events.
//
//	@Summary	List audit log
//	@Tags		identity/admin
//	@Param		resourceType	query		string	false	"resource type"
//	@Param		resourceId		query		string	false	"resource ID"
//	@Param		actorId			query		string	false	"actor ID"
//	@Param		limit			query		int		false	"page size"
//	@Param		offset			query		int		false	"offset"
//	@Success	200				{object}	httpx.ListEnvelope[handler.auditDTO]
//	@Failure	401,403			{object}	httpx.ErrorBody
//	@Router		/identity/admin/audit [get]
func (h *AdminHandler) listAudit(c echo.Context) error {
	f := model.AuditFilter{ResourceType: c.QueryParam("resourceType"), ResourceID: c.QueryParam("resourceId"), ActorID: c.QueryParam("actorId")}
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
