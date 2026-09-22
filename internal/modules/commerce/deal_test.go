package commerce_test

import (
	"net/http"
	"testing"
)

func has(actions any, want string) bool {
	for _, a := range actions.([]any) {
		if a == want {
			return true
		}
	}
	return false
}

func TestRFQQuotationAndAcceptance(t *testing.T) {
	tr := newTrade(t)
	s, b := tr.supplier, tr.buyer

	r := b.Do(http.MethodPost, "/commerce/rfqs", map[string]any{"supplierCompanyId": s.CompanyID,
		"lines": []map[string]any{{"modelId": tr.model, "quantity": "5"}}})
	expect(t, r, http.StatusCreated)
	id := str(r.Data(), "id")
	expect(t, s.Do(http.MethodGet, "/commerce/rfqs/"+id, nil), http.StatusNotFound) // drafts are private
	expect(t, b.Do(http.MethodPost, "/commerce/rfqs/"+id+"/send", map[string]any{}, ifMatch("1")...), http.StatusOK)
	if !has(s.Do(http.MethodGet, "/commerce/rfqs/"+id, nil).Data()["allowedActions"], "quote") {
		t.Fatal("supplier should be able to quote")
	}

	quote := func(price string, rev string) map[string]any {
		q := s.Do(http.MethodPost, "/commerce/rfqs/"+id+"/quotation-versions", map[string]any{"terms": map[string]any{
			"lines": []map[string]any{{"modelId": tr.model, "quantity": "5", "unitPrice": usd(price)}}, "route": "factory"}}, ifMatch(rev)...)
		expect(t, q, http.StatusCreated)
		return q.Data()
	}
	expect(t, b.Do(http.MethodPost, "/commerce/rfqs/"+id+"/quotation-versions", map[string]any{"terms": map[string]any{}}, ifMatch("2")...), http.StatusUnprocessableEntity)
	q1 := quote("1500000", "2")["quotations"].([]any)[0].(map[string]any)
	q2 := quote("1450000", "3")["quotations"].([]any)[1].(map[string]any)
	if q2["number"] != float64(2) || q2["total"].(map[string]any)["amountMinor"] != "7250000" {
		t.Fatalf("second quotation: %v", q2)
	}

	accept := func(q map[string]any, dig, rev string) (int, map[string]any) {
		res := b.Do(http.MethodPost, "/commerce/rfqs/"+id+"/accept", map[string]string{"quotationVersionId": str(q, "id"), "quotationDigest": dig}, ifMatch(rev)...)
		return res.Status, res.Body
	}
	if st, body := accept(q1, str(q1, "digest"), "4"); st != http.StatusConflict {
		t.Fatalf("accepting an older quotation: %d %v", st, body)
	}
	if st, _ := accept(q2, "deadbeef", "4"); st != http.StatusConflict {
		t.Fatal("wrong digest accepted")
	}
	o := b.Do(http.MethodPost, "/commerce/rfqs/"+id+"/accept", map[string]string{"quotationVersionId": str(q2, "id"), "quotationDigest": str(q2, "digest")}, ifMatch("4")...)
	expect(t, o, http.StatusCreated)
	if o.Data()["status"] != "accepted" || o.Data()["source"] != "rfq" || o.Data()["party"] != "buyer" {
		t.Fatalf("order: %v", o.Data())
	}
	if st, _ := accept(q2, str(q2, "digest"), "5"); st != http.StatusConflict {
		t.Fatal("an RFQ can be accepted once")
	}
	if so := s.Do(http.MethodGet, "/commerce/orders/"+str(o.Data(), "id"), nil); so.Data()["party"] != "supplier" {
		t.Fatalf("supplier view: %v", so.Data())
	}
}

func TestDirectOrderAddendaAndCancellation(t *testing.T) {
	tr := newTrade(t)
	s, b := tr.supplier, tr.buyer
	_, version := tr.publishedOffer(t)
	lines := version["terms"].(map[string]any)["lines"].([]any)
	line1 := str(lines[0].(map[string]any), "lineId")
	order := func(line, qty string) map[string]any {
		return map[string]any{"offerVersionId": str(version, "id"), "lines": []map[string]string{{"offerLineId": line, "quantity": qty}}}
	}
	expect(t, b.Do(http.MethodPost, "/commerce/orders", order(line1, "3")), http.StatusUnprocessableEntity)
	expect(t, b.Do(http.MethodPost, "/commerce/orders", order("00000000-0000-4000-8000-00000000abcd", "1")), http.StatusUnprocessableEntity)
	expect(t, s.Do(http.MethodPost, "/commerce/orders", order(line1, "1")), http.StatusUnprocessableEntity) // own offer

	o := b.Do(http.MethodPost, "/commerce/orders", order(line1, "1"))
	expect(t, o, http.StatusCreated)
	id := str(o.Data(), "id")
	terms := o.Data()["terms"].(map[string]any)
	if o.Data()["status"] != "awaiting-supplier" || len(terms["lines"].([]any)) != 1 || len(terms["paymentSchedule"].([]any)) != 0 ||
		o.Data()["total"].(map[string]any)["amountMinor"] != "1500000" {
		t.Fatalf("direct order: %v", o.Data())
	}
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+id+"/supplier-confirmations", map[string]any{}, ifMatch("1")...), http.StatusForbidden, "wrong_party")
	expect(t, s.Do(http.MethodPost, "/commerce/orders/"+id+"/supplier-confirmations", map[string]any{}, ifMatch("1")...), http.StatusOK)

	// Addendum: proposed by one side, decided by the other; original terms stay in history.
	newTerms := map[string]any{"lines": []map[string]any{{"modelId": tr.model, "quantity": "1", "unitPrice": usd("1450000")}},
		"route": "local", "paymentSchedule": []map[string]any{{"amount": usd("1450000"), "dueDate": "2026-11-01"}}}
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+id+"/addenda", map[string]any{"terms": newTerms}, ifMatch("2")...), http.StatusUnprocessableEntity)
	prop := b.Do(http.MethodPost, "/commerce/orders/"+id+"/addenda", map[string]any{"terms": newTerms, "reason": "discount agreed by phone"}, ifMatch("2")...)
	expect(t, prop, http.StatusCreated)
	ad := str(prop.Data()["addenda"].([]any)[0].(map[string]any), "id")
	expect(t, s.Do(http.MethodPost, "/commerce/orders/"+id+"/addenda", map[string]any{"terms": newTerms, "reason": "x"}, ifMatch("3")...), http.StatusConflict, "addendum_open")
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+id+"/addenda/"+ad+"/accept", map[string]any{}, ifMatch("3")...), http.StatusForbidden, "wrong_party")
	acc := s.Do(http.MethodPost, "/commerce/orders/"+id+"/addenda/"+ad+"/accept", map[string]any{}, ifMatch("3")...)
	expect(t, acc, http.StatusOK)
	if acc.Data()["total"].(map[string]any)["amountMinor"] != "1450000" {
		t.Fatalf("terms after addendum: %v", acc.Data()["total"])
	}

	// Ending the partnership blocks new deals but not existing orders.
	expect(t, s.Do(http.MethodPost, "/commerce/partnerships/"+tr.partnership+"/end", map[string]string{"reason": "paused"}, ifMatch("2")...), http.StatusOK)
	expect(t, b.Do(http.MethodPost, "/commerce/rfqs", map[string]any{"supplierCompanyId": s.CompanyID,
		"lines": []map[string]any{{"modelId": tr.model, "quantity": "1"}}}), http.StatusConflict, "partnership_required")
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+id+"/cancellations", map[string]any{}, ifMatch("4")...), http.StatusUnprocessableEntity)
	c := b.Do(http.MethodPost, "/commerce/orders/"+id+"/cancellations", map[string]string{"reason": "no longer needed"}, ifMatch("4")...)
	expect(t, c, http.StatusOK)
	if c.Data()["status"] != "cancelled" || len(c.Data()["history"].([]any)) != 5 {
		t.Fatalf("cancelled order: %v", c.Data())
	}
	expect(t, b.Do(http.MethodPost, "/commerce/orders/"+id+"/cancellations", map[string]string{"reason": "again"}, ifMatch("5")...), http.StatusConflict, "invalid_transition")
}
