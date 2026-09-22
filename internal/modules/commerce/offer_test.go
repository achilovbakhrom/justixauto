package commerce_test

import (
	"net/http"
	"testing"

	"justixauto/internal/modules/commerce"
	"justixauto/internal/modules/inventory"
	"justixauto/internal/testkit"
)

func usd(amount string) map[string]string {
	return map[string]string{"amountMinor": amount, "currency": "USD"}
}

func terms(model string, schedule ...map[string]any) map[string]any {
	return map[string]any{
		"lines": []map[string]any{
			{"modelId": model, "quantity": "2", "unitPrice": usd("1500000")},
			{"modelId": model, "quantity": "1", "unitPrice": usd("1600000")},
		},
		"route": "local", "deliveryTerms": "Tashkent yard", "warrantyTerms": "3 years", "serviceTerms": "",
		"paymentSchedule": schedule,
	}
}

// partner makes a and b active partners.
func partner(t *testing.T, a, b *testkit.Client) string {
	req := a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": b.CompanyID})
	expect(t, req, http.StatusCreated)
	id := str(req.Data(), "id")
	expect(t, b.Do(http.MethodPost, "/commerce/partnerships/"+id+"/accept", map[string]any{}, ifMatch("1")...), http.StatusOK)
	return id
}

func TestOfferPublishingAndVisibility(t *testing.T) {
	e := testkit.New(t)
	admin := e.Admin()
	s := e.CompanyUser(admin, "Supplier Motors", commerce.PermRead, commerce.PermPartnershipsManage, commerce.PermOffersManage, inventory.PermModelsEdit)
	b1 := e.CompanyUser(admin, "Buyer One", commerce.PermRead, commerce.PermPartnershipsManage)
	b2 := e.CompanyUser(admin, "Buyer Two", commerce.PermRead, commerce.PermPartnershipsManage)
	b3 := e.CompanyUser(admin, "Buyer Three", commerce.PermRead)
	partner(t, s, b1)
	p2 := partner(t, b2, s)

	model := str(s.Do(http.MethodPost, "/inventory/vehicle-models", map[string]any{"specification": map[string]any{
		"make": "Chevrolet", "model": "Tracker", "variant": "Premier", "year": 2025, "bodyType": "SUV",
		"exteriorColor": "black", "interiorColor": "black", "powertrain": "petrol 1.2T", "drivetrain": "FWD"}}).Data(), "id")

	// Validation: one currency, exact schedule, known models, route, minor units.
	bad := terms(model)
	bad["lines"].([]map[string]any)[1]["unitPrice"] = map[string]string{"amountMinor": "1600000", "currency": "EUR"}
	expect(t, s.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": bad, "audience": map[string]any{"mode": "all-active"}}), http.StatusUnprocessableEntity)
	expect(t, s.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": terms(model, map[string]any{"amount": usd("4000000"), "dueDate": "2026-10-01"}), "audience": map[string]any{"mode": "all-active"}}), http.StatusUnprocessableEntity)
	unknown := terms("00000000-0000-4000-8000-00000000abcd")
	expect(t, s.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": unknown, "audience": map[string]any{"mode": "all-active"}}), http.StatusUnprocessableEntity)
	fraction := terms(model)
	fraction["lines"].([]map[string]any)[0]["unitPrice"] = usd("15000.50")
	expect(t, s.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": fraction, "audience": map[string]any{"mode": "all-active"}}), http.StatusUnprocessableEntity)
	expect(t, b1.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": terms(model), "audience": map[string]any{"mode": "all-active"}}), http.StatusForbidden)

	schedule := []map[string]any{{"amount": usd("2300000"), "dueDate": "2026-10-01"}, {"amount": usd("2300000"), "dueDate": "2026-11-01"}}
	created := s.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": terms(model, schedule...),
		"audience": map[string]any{"mode": "selected", "partnerCompanyIds": []string{b1.CompanyID}}})
	expect(t, created, http.StatusCreated)
	id := str(created.Data(), "id")
	v1 := created.Data()["versions"].([]any)[0].(map[string]any)
	if created.Data()["status"] != "draft" || v1["total"].(map[string]any)["amountMinor"] != "4600000" || created.Data()["publishedVersion"] != nil {
		t.Fatalf("draft: %v", created.Data())
	}

	// Drafts are invisible to partners.
	expect(t, b1.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)
	pub := s.Do(http.MethodPost, "/commerce/offers/"+id+"/publish", map[string]string{"offerVersionId": str(v1, "id")}, ifMatch("1")...)
	expect(t, pub, http.StatusOK)

	// Selected audience: only Buyer One sees it, and not the audience list.
	seen := b1.Do(http.MethodGet, "/commerce/offers/"+id, nil)
	expect(t, seen, http.StatusOK)
	if _, leaked := seen.Data()["publishedVersion"].(map[string]any)["audience"]; leaked || seen.Data()["versions"] != nil {
		t.Fatalf("buyer view leaks supplier data: %v", seen.Data())
	}
	if n := len(b1.Do(http.MethodGet, "/commerce/offers?scope=available", nil).Items()); n != 1 {
		t.Fatalf("buyer one available: %d", n)
	}
	expect(t, b2.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)
	expect(t, b3.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)

	// A selected audience must be active partners.
	withB3 := s.Do(http.MethodPost, "/commerce/offers/"+id+"/versions", map[string]any{"terms": terms(model),
		"audience": map[string]any{"mode": "selected", "partnerCompanyIds": []string{b3.CompanyID}}}, ifMatch("2")...)
	expect(t, withB3, http.StatusCreated)
	v2 := withB3.Data()["versions"].([]any)[1].(map[string]any)
	expect(t, s.Do(http.MethodPost, "/commerce/offers/"+id+"/publish", map[string]string{"offerVersionId": str(v2, "id")}, ifMatch("3")...), http.StatusConflict, "partner_not_active")

	// A new version stays unpublished until published; then all active partners see it.
	v3r := s.Do(http.MethodPost, "/commerce/offers/"+id+"/versions", map[string]any{"terms": terms(model), "audience": map[string]any{"mode": "all-active"}}, ifMatch("3")...)
	expect(t, v3r, http.StatusCreated)
	v3 := v3r.Data()["versions"].([]any)[2].(map[string]any)
	if b1.Do(http.MethodGet, "/commerce/offers/"+id, nil).Data()["publishedVersion"].(map[string]any)["number"] != float64(1) {
		t.Fatal("unpublished version must not replace the published one")
	}
	expect(t, s.Do(http.MethodPost, "/commerce/offers/"+id+"/versions", map[string]any{"terms": terms(model), "audience": map[string]any{"mode": "all-active"}}, ifMatch("3")...), http.StatusPreconditionFailed)
	expect(t, s.Do(http.MethodPost, "/commerce/offers/"+id+"/publish", map[string]string{"offerVersionId": str(v3, "id")}, ifMatch("4")...), http.StatusOK)
	expect(t, b2.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusOK)
	expect(t, b3.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)

	// Ending the partnership hides the offer from that buyer.
	expect(t, b2.Do(http.MethodPost, "/commerce/partnerships/"+p2+"/end", map[string]string{"reason": "stopped"}, ifMatch("2")...), http.StatusOK)
	expect(t, b2.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)

	// Withdrawal is final and hides the offer from everyone else.
	expect(t, s.Do(http.MethodPost, "/commerce/offers/"+id+"/withdraw", map[string]string{"reason": ""}, ifMatch("5")...), http.StatusUnprocessableEntity)
	w := s.Do(http.MethodPost, "/commerce/offers/"+id+"/withdraw", map[string]string{"reason": "sold out"}, ifMatch("5")...)
	expect(t, w, http.StatusOK)
	if w.Data()["status"] != "withdrawn" || len(w.Data()["allowedActions"].([]any)) != 0 {
		t.Fatalf("withdrawn: %v", w.Data())
	}
	expect(t, b1.Do(http.MethodGet, "/commerce/offers/"+id, nil), http.StatusNotFound)
	expect(t, s.Do(http.MethodPost, "/commerce/offers/"+id+"/publish", map[string]string{"offerVersionId": str(v3, "id")}, ifMatch("6")...), http.StatusConflict, "offer_withdrawn")
	expect(t, b1.Do(http.MethodPost, "/commerce/offers/"+id+"/withdraw", map[string]string{"reason": "x"}, ifMatch("6")...), http.StatusForbidden)
	if n := len(s.Do(http.MethodGet, "/commerce/offers?scope=own", nil).Items()); n != 1 {
		t.Fatalf("own offers: %d", n)
	}
}
