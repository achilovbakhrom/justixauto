package insurance_test

import (
	"net/http"
	"testing"
	"time"

	"justixauto/internal/e2e"
	"justixauto/internal/modules/insurance"
)

var (
	expect  = e2e.Expect
	ifMatch = e2e.IfMatch
)

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func TestInsuranceDecisionUnlocksOwnInstallmentDelivery(t *testing.T) {
	e := e2e.New(t)
	admin := e.Admin()
	shop := e.NewShop(admin, "Seller", "XTAAA11111A40000", 2, insurance.PermRead, insurance.PermApply)
	insurer := e.KindUser(admin, "insurance", "Safe Insurance", insurance.PermRead, insurance.PermReview, insurance.PermDecide)
	bank := e.KindUser(admin, "bank", "Some Bank", insurance.PermRead)

	cash := str(shop.Sale(0, "cash", "1600000").Data(), "id")
	deal := shop.Sale(1, "own-installment", "1800000")
	dealID := str(deal.Data(), "id")

	create := func(dealID, insurerID string) e2e.Response {
		return shop.Do(http.MethodPost, "/insurance/applications", map[string]string{"retailDealId": dealID, "insurerCompanyId": insurerID})
	}
	expect(t, create(cash, insurer.CompanyID), http.StatusConflict, "sale_not_eligible")
	expect(t, create(dealID, bank.CompanyID), http.StatusUnprocessableEntity)
	a := create(dealID, insurer.CompanyID)
	expect(t, a, http.StatusCreated)
	id := str(a.Data(), "id")
	expect(t, create(dealID, insurer.CompanyID), http.StatusConflict, "application_exists")

	// Drafts are invisible to the insurer.
	expect(t, insurer.Do(http.MethodGet, "/insurance/applications/"+id, nil), http.StatusNotFound)
	if n := len(insurer.Do(http.MethodGet, "/insurance/applications", nil).Items()); n != 0 {
		t.Fatalf("insurer sees drafts: %d", n)
	}
	expect(t, shop.Do(http.MethodPost, "/insurance/applications/"+id+"/submit", map[string]any{"confirmation": true, "dealRevision": "99"}, ifMatch("1")...), http.StatusConflict, "sale_changed")
	expect(t, shop.Do(http.MethodPost, "/insurance/applications/"+id+"/submit", map[string]any{"confirmation": true, "dealRevision": deal.Revision()}, ifMatch("1")...), http.StatusOK)
	expect(t, shop.Do(http.MethodPatch, "/insurance/applications/"+id, map[string]string{"insurerCompanyId": insurer.CompanyID}, ifMatch("2")...), http.StatusConflict, "not_draft")

	// Only the addressed insurer reviews; notes are required.
	act := func(c *e2e.Client, path, note, rev string) e2e.Response {
		return c.Do(http.MethodPost, "/insurance/applications/"+id+"/"+path, map[string]string{"note": note}, ifMatch(rev)...)
	}
	expect(t, act(shop.Client, "take", "", "2"), http.StatusForbidden)
	taken := act(insurer, "take", "", "2")
	expect(t, taken, http.StatusOK)
	// The insurer sees the vehicle and the client from the submitted snapshot.
	snap := taken.Data()["snapshot"].(map[string]any)
	if snap["vehicle"].(map[string]any)["vin"] != "XTAAA11111A40000B" || snap["customer"].(map[string]any)["name"] == "" {
		t.Fatalf("snapshot: %v", snap)
	}
	expect(t, act(insurer, "information-requests", "", "3"), http.StatusUnprocessableEntity)
	expect(t, act(insurer, "information-requests", "Need the customer's income statement", "3"), http.StatusOK)
	expect(t, act(shop.Client, "responses", "Uploaded to the deal file", "4"), http.StatusOK)

	approved := act(insurer, "approve", "Risk acceptable", "5")
	expect(t, approved, http.StatusOK)
	h := approved.Data()["history"].([]any)
	if approved.Data()["status"] != "approved" || len(h) != 5 || h[3].(map[string]any)["requestId"] == nil {
		t.Fatalf("approved: %v", approved.Data())
	}

	// The approval is one of the own-installment delivery prerequisites.
	rev := func() string { return shop.Do(http.MethodGet, "/retail/deals/"+dealID, nil).Revision() }
	if shop.Do(http.MethodGet, "/retail/deals/"+dealID, nil).Data()["checklist"].(map[string]any)["insuranceApproved"] != true {
		t.Fatal("checklist should show the approval")
	}
	pay := func(purpose string, amount map[string]string) {
		inv := shop.Do(http.MethodPost, "/retail/deals/"+dealID+"/invoices", map[string]any{"purpose": purpose, "amount": amount, "recipientSnapshot": "Seller"}, ifMatch(rev())...)
		expect(t, inv, http.StatusCreated)
		ev := shop.Do(http.MethodPost, "/retail/invoices/"+str(inv.Data(), "id")+"/evidence", map[string]any{"claimedAmount": amount, "paidOn": "2026-09-22", "externalReference": "R-" + purpose})
		expect(t, shop.Do(http.MethodPost, "/retail/evidence/"+str(ev.Data()["paymentEvidence"].([]any)[0].(map[string]any), "id")+"/accept", map[string]any{"confirmation": true}, ifMatch("1")...), http.StatusOK)
	}
	pay("first-installment", map[string]string{"amountMinor": "300000", "currency": "USD"})
	expect(t, shop.Do(http.MethodPost, "/retail/deals/"+dealID+"/contract-records", map[string]string{"signedOn": "2026-09-22", "reference": "INST-1"}, ifMatch(rev())...), http.StatusOK)
	pay("registration", map[string]string{"amountMinor": "150000000", "currency": "UZS"})
	expect(t, shop.Do(http.MethodPost, "/retail/deals/"+dealID+"/registration", map[string]string{"registeredOn": "2026-09-22", "plateNumber": "01B777AA"}, ifMatch(rev())...), http.StatusOK)
	expect(t, shop.Do(http.MethodPost, "/retail/deals/"+dealID+"/deliveries", map[string]string{"occurredAt": e.Clock.Now().Format(time.RFC3339)}, ifMatch(rev())...), http.StatusOK)
}
