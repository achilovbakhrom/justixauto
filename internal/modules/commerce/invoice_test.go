package commerce_test

import (
	"net/http"
	"testing"
)

func TestInvoicesAndPaymentEvidence(t *testing.T) {
	tr := newTrade(t)
	s, b := tr.supplier, tr.buyer
	_, version := tr.publishedOffer(t)
	lines := version["terms"].(map[string]any)["lines"].([]any)
	o := b.Do(http.MethodPost, "/commerce/orders", map[string]any{"offerVersionId": str(version, "id"), "lines": []map[string]string{
		{"offerLineId": str(lines[0].(map[string]any), "lineId"), "quantity": "2"}, {"offerLineId": str(lines[1].(map[string]any), "lineId"), "quantity": "1"}}})
	expect(t, o, http.StatusCreated)
	order := str(o.Data(), "id")
	expect(t, s.Do(http.MethodPost, "/commerce/orders/"+order+"/supplier-confirmations", map[string]any{}, ifMatch("1")...), http.StatusOK)

	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+order+"/invoices", map[string]any{}, ifMatch("2")...), http.StatusForbidden, "wrong_party")
	inv := s.Do(http.MethodPost, "/commerce/orders/"+order+"/invoices", map[string]any{}, ifMatch("2")...)
	expect(t, inv, http.StatusCreated)
	id := str(inv.Data(), "id")
	if inv.Data()["total"].(map[string]any)["amountMinor"] != "4600000" || len(inv.Data()["schedule"].([]any)) != 1 {
		t.Fatalf("invoice: %v", inv.Data())
	}
	expect(t, s.Do(http.MethodPost, "/commerce/orders/"+order+"/invoices", map[string]any{}, ifMatch("3")...), http.StatusConflict, "invoice_exists")

	pay := func(amount, currency, paidOn string) (int, map[string]any) {
		r := b.Do(http.MethodPost, "/commerce/invoices/"+id+"/payment-evidence", map[string]any{
			"claimedAmount": map[string]string{"amountMinor": amount, "currency": currency}, "paidOn": paidOn, "externalReference": "PP-" + amount})
		return r.Status, r.Data()
	}
	for _, bad := range [][3]string{{"5000000", "USD", "2026-09-20"}, {"1000", "EUR", "2026-09-20"}, {"1000", "USD", "2027-01-01"}, {"10.5", "USD", "2026-09-20"}} {
		if st, _ := pay(bad[0], bad[1], bad[2]); st != http.StatusUnprocessableEntity {
			t.Fatalf("%v accepted with %d", bad, st)
		}
	}
	if st, d := pay("3000000", "USD", "2026-09-20"); st != http.StatusCreated || d["pending"].(map[string]any)["amountMinor"] != "3000000" {
		t.Fatalf("first claim: %d %v", st, d)
	}
	if st, _ := pay("2000000", "USD", "2026-09-21"); st != http.StatusUnprocessableEntity {
		t.Fatal("claims above the open amount must be refused")
	}
	_, d := pay("1600000", "USD", "2026-09-21")
	ev := d["paymentEvidence"].([]any)
	first, second := ev[0].(map[string]any), ev[1].(map[string]any)
	expect(t, s.Do(http.MethodPost, "/commerce/invoices/"+id+"/void", map[string]string{"reason": "x"}, ifMatch("1")...), http.StatusConflict, "invoice_has_payments")

	// Accepting a payment is sensitive: it needs a fresh second factor.
	accept := func(e map[string]any, body map[string]any) (int, map[string]any) {
		r := s.Do(http.MethodPost, "/commerce/payment-evidence/"+str(e, "id")+"/accept", body, ifMatch(str(e, "revision"))...)
		return r.Status, r.Body
	}
	if st, body := accept(first, map[string]any{"confirmation": true}); st != http.StatusForbidden ||
		body["error"].(map[string]any)["code"] != "mfa_enrollment_required" {
		t.Fatalf("accept without MFA: %d %v", st, body)
	}
	s.EnrollMFA()
	if st, body := accept(first, map[string]any{}); st != http.StatusUnprocessableEntity {
		t.Fatalf("acceptance needs explicit confirmation: %d %v", st, body)
	}
	expect(t, b.Do(http.MethodPost, "/commerce/payment-evidence/"+str(first, "id")+"/accept", map[string]any{"confirmation": true}, ifMatch("1")...), http.StatusForbidden)
	st, body := accept(first, map[string]any{"confirmation": true})
	if st != http.StatusOK || body["data"].(map[string]any)["paid"].(map[string]any)["amountMinor"] != "3000000" {
		t.Fatalf("accepted: %d %v", st, body)
	}
	rej := s.Do(http.MethodPost, "/commerce/payment-evidence/"+str(second, "id")+"/reject", map[string]string{"reason": "not on our statement"}, ifMatch("1")...)
	expect(t, rej, http.StatusOK)
	if rej.Data()["outstanding"].(map[string]any)["amountMinor"] != "1600000" || !has(b.Do(http.MethodGet, "/commerce/invoices/"+id, nil).Data()["allowedActions"], "submit-payment") {
		t.Fatalf("after decisions: %v", rej.Data())
	}
	rev := b.Do(http.MethodGet, "/commerce/orders/"+order, nil).Revision()
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+order+"/cancellations", map[string]string{"reason": "changed mind"}, ifMatch(rev)...), http.StatusConflict, "cancellation_blocked")
}
