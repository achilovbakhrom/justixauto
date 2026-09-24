package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/modules/commerce/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

type partnershipDTO struct {
	ID             string                  `json:"id"`
	Direction      string                  `json:"direction"` // outgoing: we asked; incoming: they asked
	Counterparty   counterpartyDTO         `json:"counterparty"`
	Status         model.PartnershipStatus `json:"status"`
	StatusReason   string                  `json:"statusReason"`
	AllowedActions []string                `json:"allowedActions"`
	Revision       string                  `json:"revision"`
	CreatedAt      time.Time               `json:"createdAt"`
	ActivatedAt    *time.Time              `json:"activatedAt"`
	ClosedAt       *time.Time              `json:"closedAt"`
}

func toPartnership(companyID string) func(*service.PartnershipView) partnershipDTO {
	return func(v *service.PartnershipView) partnershipDTO {
		p := v.Partnership
		dir := "incoming"
		if p.RequesterCompanyID == companyID {
			dir = "outgoing"
		}
		return partnershipDTO{
			ID: p.ID, Direction: dir,
			Counterparty: counterpartyDTO{ID: v.Counterparty.ID, Name: v.Counterparty.Name, Country: v.Counterparty.Country},
			Status:       p.Status, StatusReason: p.StatusReason, AllowedActions: p.AllowedActions(companyID),
			Revision: httpx.Revision(p.Version), CreatedAt: p.CreatedAt, ActivatedAt: p.ActivatedAt, ClosedAt: p.ClosedAt,
		}
	}
}

// requestPartnershipRequest requests a trading partnership with another company.
type requestPartnershipRequest struct {
	CounterpartyCompanyID string `json:"counterpartyCompanyId"`
}

// partnershipDecisionRequest decides an incoming/outgoing partnership request.
type partnershipDecisionRequest struct {
	Reason string `json:"reason"`
}

// listPartnerships lists trading partnerships for the active company.
//
//	@Summary	List partnerships
//	@Tags		commerce/partnerships
//	@Param		status		query		string	false	"partnership status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.partnershipDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/commerce/partnerships [get]
func (h *Handler) listPartnerships(c echo.Context) error {
	p := auth.Get(c)
	f := model.PartnershipFilter{Status: model.PartnershipStatus(c.QueryParam("status"))}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	views, err := h.partnerships.List(c.Request().Context(), p, f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(views, toPartnership(p.CompanyID)), nil)
}

// getPartnership returns one partnership.
//
//	@Summary	Get partnership
//	@Tags		commerce/partnerships
//	@Param		id			path		string	true	"partnership ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.partnershipDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/partnerships/{id} [get]
func (h *Handler) getPartnership(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.partnerships.Get(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}

// requestPartnership asks another company to become a trading partner.
//
//	@Summary	Request partnership
//	@Tags		commerce/partnerships
//	@Security	CSRF
//	@Param		body				body		requestPartnershipRequest	true	"counterparty"
//	@Success	201					{object}	httpx.DataEnvelope[handler.partnershipDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/commerce/partnerships [post]
func (h *Handler) requestPartnership(c echo.Context) error {
	p := auth.Get(c)
	var in requestPartnershipRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.partnerships.Request(c.Request().Context(), p, in.CounterpartyCompanyID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}

// decidePartnership accepts/rejects/closes a partnership request (path action).
//
//	@Summary	Decide partnership
//	@Tags		commerce/partnerships
//	@Security	CSRF
//	@Param		id							path		string						true	"partnership ID"
//	@Param		action						path		string						true	"decision action"
//	@Param		If-Match					header		string						true	"revision"
//	@Param		body						body		partnershipDecisionRequest	true	"reason"
//	@Success	200							{object}	httpx.DataEnvelope[handler.partnershipDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/partnerships/{id}/{action} [post]
func (h *Handler) decidePartnership(c echo.Context) error {
	p := auth.Get(c)
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in partnershipDecisionRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.partnerships.Decide(c.Request().Context(), p, c.Param("id"), expected, c.Param("action"), in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}
