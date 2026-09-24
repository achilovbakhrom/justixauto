package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/modules/commerce/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/money"
)

type offerVersionDTO struct {
	ID          string          `json:"id"`
	Number      int             `json:"number"`
	Terms       model.Terms     `json:"terms"`
	Total       money.Money     `json:"total"`
	Audience    *model.Audience `json:"audience,omitempty"` // supplier only
	CreatedAt   time.Time       `json:"createdAt"`
	PublishedAt *time.Time      `json:"publishedAt"`
}

func toOfferVersion(v *model.OfferVersion, own bool) offerVersionDTO {
	terms, audience := v.Decode()
	d := offerVersionDTO{ID: v.ID, Number: v.Number, Terms: terms, Total: model.TermsTotal(terms), CreatedAt: v.CreatedAt, PublishedAt: v.PublishedAt}
	if own {
		if audience.PartnerCompanyIDs == nil {
			audience.PartnerCompanyIDs = []string{}
		}
		d.Audience = &audience
	}
	return d
}

type offerDTO struct {
	ID               string            `json:"id"`
	Supplier         counterpartyDTO   `json:"supplier"`
	Status           model.OfferStatus `json:"status"`
	StatusReason     string            `json:"statusReason"`
	PublishedVersion *offerVersionDTO  `json:"publishedVersion"`
	Versions         []offerVersionDTO `json:"versions,omitempty"` // supplier only
	AllowedActions   []string          `json:"allowedActions"`
	Revision         string            `json:"revision"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

func toOffer(v *service.OfferView) offerDTO {
	o := v.Offer
	d := offerDTO{
		ID: o.ID, Supplier: counterpartyDTO{ID: v.Supplier.ID, Name: v.Supplier.Name, Country: v.Supplier.Country},
		Status: o.Status, StatusReason: o.StatusReason, AllowedActions: []string{}, Revision: httpx.Revision(o.Version), UpdatedAt: o.UpdatedAt,
	}
	if v.Published != nil {
		pv := toOfferVersion(v.Published, v.Own)
		d.PublishedVersion = &pv
	}
	for i := range v.Versions {
		d.Versions = append(d.Versions, toOfferVersion(&v.Versions[i], true))
	}
	if v.Own && o.Status != model.OfferWithdrawn {
		d.AllowedActions = []string{"add-version", "publish", "withdraw"}
	}
	return d
}

// publishOfferRequest publishes a specific offer version.
type publishOfferRequest struct {
	OfferVersionID string `json:"offerVersionId"`
}

// offerWithdrawRequest withdraws an offer.
type offerWithdrawRequest struct {
	Reason string `json:"reason"`
}

// listOffers lists supplier offers visible to the active company.
//
//	@Summary	List offers
//	@Tags		commerce/offers
//	@Param		scope		query		string	false	"offer scope (own, partners)"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.offerDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/commerce/offers [get]
func (h *Handler) listOffers(c echo.Context) error {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := httpx.IntQuery(c, "offset")
	if err != nil {
		return err
	}
	views, err := h.offers.List(c.Request().Context(), auth.Get(c), c.QueryParam("scope"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(views, toOffer), nil)
}

// getOffer returns one offer.
//
//	@Summary	Get offer
//	@Tags		commerce/offers
//	@Param		id			path		string	true	"offer ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.offerDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/offers/{id} [get]
func (h *Handler) getOffer(c echo.Context) error {
	v, err := h.offers.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}

// createOffer creates a supplier offer.
//
//	@Summary	Create offer
//	@Tags		commerce/offers
//	@Security	CSRF
//	@Param		body		body		service.OfferInput	true	"offer"
//	@Success	201			{object}	httpx.DataEnvelope[handler.offerDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/commerce/offers [post]
func (h *Handler) createOffer(c echo.Context) error {
	var in service.OfferInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toOffer(v), v.Offer.Version)
}

// addOfferVersion adds a new terms version to an offer.
//
//	@Summary	Add offer version
//	@Tags		commerce/offers
//	@Security	CSRF
//	@Param		id							path		string				true	"offer ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		service.OfferInput	true	"offer"
//	@Success	201							{object}	httpx.DataEnvelope[handler.offerDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/offers/{id}/versions [post]
func (h *Handler) addOfferVersion(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.OfferInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.AddVersion(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toOffer(v), v.Offer.Version)
}

// publishOffer publishes an offer version, replacing the currently published one.
//
//	@Summary	Publish offer
//	@Tags		commerce/offers
//	@Security	CSRF
//	@Param		id							path		string				true	"offer ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		publishOfferRequest	true	"offer version"
//	@Success	200							{object}	httpx.DataEnvelope[handler.offerDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/offers/{id}/publish [post]
func (h *Handler) publishOffer(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in publishOfferRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.Publish(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.OfferVersionID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}

// withdrawOffer withdraws an offer so it can no longer be traded on.
//
//	@Summary	Withdraw offer
//	@Tags		commerce/offers
//	@Security	CSRF
//	@Param		id						path		string					true	"offer ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		offerWithdrawRequest	true	"reason"
//	@Success	200						{object}	httpx.DataEnvelope[handler.offerDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/offers/{id}/withdraw [post]
func (h *Handler) withdrawOffer(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in offerWithdrawRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.Withdraw(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}
