package financing_test

import (
	"net/http"
	"testing"

	"justixauto/internal/modules/documents"
	"justixauto/internal/modules/financing"
	"justixauto/internal/testkit"
)

var (
	expect  = testkit.Expect
	ifMatch = testkit.IfMatch
)

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func program(policy string) map[string]any {
	return map[string]any{"name": "Auto 24", "currency": "USD", "calculationPolicyId": policy, "calculationPolicyVersion": 1,
		"terms":       map[string]any{"markupBps": 1500, "minDownPaymentBps": 2000, "termMonths": []int{12, 24}},
		"eligibility": map[string]string{"maxPriceMinor": "10000000"}}
}

func TestProgramApplicationTermsAndAgreement(t *testing.T) {
	e := testkit.New(t)
	admin := e.Admin()
	bank := e.KindUser(admin, "bank", "Capital Bank", financing.PermRead, financing.PermPrograms, financing.PermReview, financing.PermDecide,
		documents.PermRead, documents.PermSensitiveDownload)
	shop := e.NewShop(admin, "Seller", "XTAAA11111A50000", 1, financing.PermRead, financing.PermApply, financing.PermAgree,
		documents.PermRead, documents.PermUpload)

	// Programs: only the documented policy; drafts are private.
	expect(t, bank.Do(http.MethodPost, "/financing/programs", program("guess")), http.StatusUnprocessableEntity)
	p := bank.Do(http.MethodPost, "/financing/programs", program("fixed-markup"))
	expect(t, p, http.StatusCreated)
	prog := str(p.Data(), "id")
	if n := len(shop.Do(http.MethodGet, "/financing/programs?providerId="+bank.CompanyID, nil).Items()); n != 0 {
		t.Fatalf("seller sees draft programs: %d", n)
	}
	expect(t, bank.Do(http.MethodPost, "/financing/programs/"+prog+"/publish", map[string]int{"programVersion": 1}, ifMatch("1")...), http.StatusOK)
	if n := len(shop.Do(http.MethodGet, "/financing/programs?providerId="+bank.CompanyID, nil).Items()); n != 1 {
		t.Fatalf("published programs: %d", n)
	}

	deal := shop.Sale(0, "partner-finance", "2000000")
	calc := map[string]any{"downPayment": map[string]string{"amountMinor": "500000", "currency": "USD"}, "termMonths": 12, "firstDueDate": "2026-10-15"}
	a := shop.Do(http.MethodPost, "/financing/applications", map[string]any{"retailDealId": str(deal.Data(), "id"),
		"providerCompanyId": bank.CompanyID, "programId": prog, "programVersion": 1, "calculationInputs": calc})
	expect(t, a, http.StatusCreated)
	id := str(a.Data(), "id")
	out := a.Data()["calculation"].(map[string]any)["output"].(map[string]any)
	// 2000000 + 15% markup = 2300000; financed 1800000 over 12 months.
	if out["total"].(map[string]any)["amountMinor"] != "2300000" || len(out["schedule"].([]any)) != 12 {
		t.Fatalf("calculation: %v", out)
	}
	expect(t, bank.Do(http.MethodGet, "/financing/applications/"+id, nil), http.StatusNotFound) // draft

	submit := func(digest string) testkit.Response {
		return shop.Do(http.MethodPost, "/financing/applications/"+id+"/submit", map[string]any{"confirmation": true,
			"dealRevision": deal.Revision(), "calculationDigest": digest}, ifMatch("1")...)
	}
	expect(t, submit("stale"), http.StatusConflict, "calculation_changed")
	expect(t, submit(str(a.Data(), "calculationDigest")), http.StatusOK)

	act := func(c *testkit.Client, path string, body map[string]any, rev string) testkit.Response {
		return c.Do(http.MethodPost, "/financing/applications/"+id+"/"+path, body, ifMatch(rev)...)
	}
	expect(t, act(shop.Client, "take", nil, "2"), http.StatusForbidden)
	taken := act(bank, "take", nil, "2")
	expect(t, taken, http.StatusOK)
	// The provider sees the vehicle and the client from the submitted snapshot.
	snap := taken.Data()["snapshot"].(map[string]any)
	if v := snap["vehicle"].(map[string]any); v["vin"] == "" || v["model"] == "" || snap["customer"].(map[string]any)["name"] == "" {
		t.Fatalf("snapshot: %v", snap)
	}
	expect(t, act(bank, "information-requests", map[string]any{"note": "Customer's employment certificate"}, "3"), http.StatusOK)
	expect(t, act(shop.Client, "responses", map[string]any{"note": "Sent by email"}, "4"), http.StatusOK)

	// Proposing terms is a sensitive decision; the server recalculates.
	terms := map[string]any{"note": "We need 30% down", "calculationInputs": map[string]any{"downPayment": map[string]string{"amountMinor": "600000", "currency": "USD"}, "termMonths": 24, "firstDueDate": "2026-10-15"}}
	expect(t, act(bank, "terms", terms, "5"), http.StatusForbidden, "mfa_enrollment_required")
	bank.EnrollMFA()
	t1 := act(bank, "terms", terms, "5")
	expect(t, t1, http.StatusOK)
	if t1.Data()["status"] != "terms" || len(t1.Data()["terms"].([]any)) != 1 {
		t.Fatalf("terms: %v", t1.Data())
	}
	expect(t, act(shop.Client, "counter", map[string]any{"termsVersion": 1, "note": "Customer asks for 20% down"}, "6"), http.StatusOK)
	expect(t, act(bank, "terms", map[string]any{"note": "25% down, 24 months", "calculationInputs": map[string]any{
		"downPayment": map[string]string{"amountMinor": "500000", "currency": "USD"}, "termMonths": 24, "firstDueDate": "2026-10-15"}}, "7"), http.StatusOK)

	// The seller agrees to the exact current terms, with a fresh second factor.
	expect(t, act(shop.Client, "agree", map[string]any{"termsVersion": 1, "confirmation": true}, "8"), http.StatusForbidden)
	shop.EnrollMFA()
	expect(t, act(shop.Client, "agree", map[string]any{"termsVersion": 1, "confirmation": true}, "8"), http.StatusConflict, "terms_changed")
	agreed := act(shop.Client, "agree", map[string]any{"termsVersion": 2, "confirmation": true}, "8")
	expect(t, agreed, http.StatusOK)
	if agreed.Data()["status"] != "agreed" || len(agreed.Data()["history"].([]any)) != 8 {
		t.Fatalf("agreed: %v", agreed.Data())
	}
	// Agreement is not delivery: partner-finance delivery stays policy-gated.
	expect(t, shop.Do(http.MethodPost, "/retail/deals/"+str(deal.Data(), "id")+"/deliveries", map[string]string{"occurredAt": "2026-09-22T09:00:00Z"}, ifMatch(deal.Revision())...), http.StatusForbidden, "policy_unresolved")

	// After agreement: document requests with numbered submissions.
	appID := id
	req := bank.Do(http.MethodPost, "/financing/applications/"+appID+"/document-requests", map[string]string{"title": "Signed purchase agreement", "requirements": "PDF, all pages"})
	expect(t, req, http.StatusCreated)
	doc := str(req.Data(), "id")
	other := e.CompanyUser(admin, "Other", documents.PermUpload)
	foreign := str(other.Upload("finance-document", "x.pdf", testkit.PDF).Data(), "id")
	expect(t, shop.Do(http.MethodPost, "/financing/document-requests/"+doc+"/submissions", map[string]string{"attachmentBindingId": foreign}, ifMatch("1")...), http.StatusUnprocessableEntity)
	file := str(shop.Upload("finance-document", "agreement.pdf", testkit.PDF).Data(), "id")
	if status, _ := bank.Raw("/documents/files/" + file + "/content"); status != http.StatusNotFound {
		t.Fatal("the provider must not read the file before it is submitted")
	}
	expect(t, shop.Do(http.MethodPost, "/financing/document-requests/"+doc+"/submissions", map[string]string{"attachmentBindingId": file, "note": "v1"}, ifMatch("1")...), http.StatusCreated)
	if status, _ := bank.Raw("/documents/files/" + file + "/content"); status != http.StatusOK {
		t.Fatalf("provider download after submission: %d", status)
	}
	expect(t, bank.Do(http.MethodPost, "/financing/document-requests/"+doc+"/return", map[string]any{"submissionVersion": 1}, ifMatch("2")...), http.StatusUnprocessableEntity)
	expect(t, bank.Do(http.MethodPost, "/financing/document-requests/"+doc+"/return", map[string]any{"submissionVersion": 1, "note": "Page 3 missing"}, ifMatch("2")...), http.StatusOK)
	expect(t, shop.Do(http.MethodPost, "/financing/document-requests/"+doc+"/submissions", map[string]string{"attachmentBindingId": file, "note": "v2"}, ifMatch("3")...), http.StatusCreated)
	expect(t, bank.Do(http.MethodPost, "/financing/document-requests/"+doc+"/accept", map[string]any{"submissionVersion": 1, "confirmation": true}, ifMatch("4")...), http.StatusConflict, "submission_changed")
	acc := bank.Do(http.MethodPost, "/financing/document-requests/"+doc+"/accept", map[string]any{"submissionVersion": 2, "confirmation": true}, ifMatch("4")...)
	expect(t, acc, http.StatusOK)
	if acc.Data()["status"] != "accepted" || len(acc.Data()["submissions"].([]any)) != 2 {
		t.Fatalf("document request: %v", acc.Data())
	}
}
