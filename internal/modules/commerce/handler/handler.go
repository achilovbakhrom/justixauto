package handler

import (
	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/modules/commerce/service"
	"justixauto/internal/pkg/auth"
)

// Handler mounts commerce's Echo routes.
type Handler struct {
	partnerships *service.Partnership
	offers       *service.Offer
	deals        *service.Deal
	fulfilment   *service.Fulfilment
	invoices     *service.Invoice
}

// New builds the commerce handler.
func New(partnerships *service.Partnership, offers *service.Offer, deals *service.Deal, fulfilment *service.Fulfilment, invoices *service.Invoice) *Handler {
	return &Handler{partnerships: partnerships, offers: offers, deals: deals, fulfilment: fulfilment, invoices: invoices}
}

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/partnerships", h.listPartnerships, auth.Require(model.PermRead))
	c.GET("/partnerships/:id", h.getPartnership, auth.Require(model.PermRead))
	c.POST("/partnerships", h.requestPartnership, auth.Require(model.PermPartnershipsManage))
	c.POST("/partnerships/:id/:action", h.decidePartnership, auth.Require(model.PermPartnershipsManage))

	c.GET("/offers", h.listOffers, auth.Require(model.PermRead))
	c.GET("/offers/:id", h.getOffer, auth.Require(model.PermRead))
	c.POST("/offers", h.createOffer, auth.Require(model.PermOffersManage))
	c.POST("/offers/:id/versions", h.addOfferVersion, auth.Require(model.PermOffersManage))
	c.POST("/offers/:id/publish", h.publishOffer, auth.Require(model.PermOffersManage))
	c.POST("/offers/:id/withdraw", h.withdrawOffer, auth.Require(model.PermOffersManage))

	h.dealRoutes(c)
	h.invoiceRoutes(c)
}
