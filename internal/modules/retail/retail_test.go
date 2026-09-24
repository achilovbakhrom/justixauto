package retail_test

import (
	"net/http"
	"testing"
	"time"

	"justixauto/internal/e2e"
	"justixauto/internal/modules/documents"
	"justixauto/internal/modules/inventory"
	"justixauto/internal/modules/retail"
)

var (
	expect  = e2e.Expect
	ifMatch = e2e.IfMatch
)

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

type shop struct {
	e        *e2e.Env
	c        *e2e.Client
	branch   string
	vehicles []string
}

var perms = []string{
	retail.PermRead, retail.PermCRM, retail.PermListings, retail.PermDeals, retail.PermPaymentsAccept, retail.PermDeliver,
	inventory.PermRead, inventory.PermModelsEdit, inventory.PermWarehousesManage, inventory.PermReceiptsCreate, documents.PermUpload, documents.PermRead,
}

func newShop(t *testing.T) *shop {
	e := e2e.New(t)
	admin := e.Admin()
	c := e.CompanyUser(admin, "Retail Motors", perms...)
	br := c.Do(http.MethodPost, "/identity/companies/"+c.CompanyID+"/branches", map[string]any{"name": "Chilanzar"})
	expect(t, br, http.StatusCreated)
	model := str(c.Do(http.MethodPost, "/inventory/vehicle-models", map[string]any{"specification": map[string]any{
		"make":  "Chevrolet",
		"model": "Cobalt", "variant": "LTZ", "year": 2025, "bodyType": "sedan", "exteriorColor": "white", "interiorColor": "black",
		"powertrain": "petrol", "drivetrain": "FWD",
	}}).Data(), "id")
	w := c.Do(http.MethodPost, "/inventory/warehouses", map[string]any{
		"name": "Showroom", "country": map[string]string{"label": "Uzbekistan"},
		"city": "Tashkent", "address": "1", "capacity": "10",
	})
	r := c.Do(http.MethodPost, "/inventory/warehouses/"+str(w.Data(), "id")+"/receipt-batches", map[string]any{
		"modelId":                   model,
		"modelSpecificationVersion": "1", "stock": map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A300001", "XTAAA11111A300002", "XTAAA11111A300003"}},
		"receivedAt": e.Clock.Now().Format(time.RFC3339),
	}, ifMatch("1")...)
	expect(t, r, http.StatusCreated)
	s := &shop{e: e, c: c, branch: str(br.Data(), "id")}
	for _, v := range r.Data()["vehicles"].([]any) {
		s.vehicles = append(s.vehicles, str(v.(map[string]any), "id"))
	}
	return s
}

func (s *shop) customer(t *testing.T, name string) string {
	cu := s.c.Do(http.MethodPost, "/retail/customers", map[string]any{"profile": map[string]string{"displayName": name, "phone": "+998901234567"}})
	expect(t, cu, http.StatusCreated)
	return str(cu.Data(), "id")
}

func usd(a string) map[string]string { return map[string]string{"amountMinor": a, "currency": "USD"} }

func TestLeadsTasksAndListings(t *testing.T) {
	s := newShop(t)
	c := s.c
	cust := s.customer(t, "Aziz")
	expect(t, c.Do(http.MethodPost, "/retail/leads", map[string]any{"customerId": cust, "branchId": s.branch, "source": "instagram"}), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, "/retail/leads", map[string]any{"customerId": cust, "branchId": c.CompanyID, "source": "phone"}), http.StatusUnprocessableEntity)
	l := c.Do(http.MethodPost, "/retail/leads", map[string]any{"customerId": cust, "branchId": s.branch, "source": "phone"})
	expect(t, l, http.StatusCreated)
	id := str(l.Data(), "id")
	stage := func(st, reason, rev string) e2e.Response {
		return c.Do(http.MethodPost, "/retail/leads/"+id+"/stage", map[string]string{"stage": st, "reason": reason}, ifMatch(rev)...)
	}
	expect(t, stage("qualified", "", "1"), http.StatusUnprocessableEntity) // no skipping
	expect(t, stage("won", "", "1"), http.StatusUnprocessableEntity)       // only by delivery
	expect(t, stage("contacted", "", "1"), http.StatusOK)
	expect(t, stage("lost", "", "2"), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, "/retail/leads/"+id+"/assign", map[string]string{"assignedUserId": "00000000-0000-4000-8000-00000000abcd"}, ifMatch("2")...), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, "/retail/leads/"+id+"/assign", map[string]string{"assignedUserId": c.UserID}, ifMatch("2")...), http.StatusOK)
	expect(t, c.Do(http.MethodPost, "/retail/leads/"+id+"/contacts", map[string]string{"channel": "phone", "note": "wants a white Cobalt"}), http.StatusCreated)
	detail := c.Do(http.MethodGet, "/retail/leads/"+id, nil)
	if len(detail.Data()["contacts"].([]any)) != 1 || len(detail.Data()["history"].([]any)) != 3 {
		t.Fatalf("lead detail: %v", detail.Data())
	}

	task := c.Do(http.MethodPost, "/retail/tasks", map[string]any{
		"customerId": cust, "leadId": id, "ownerUserId": c.UserID,
		"dueAt": s.e.Clock.Now().Add(24 * time.Hour).Format(time.RFC3339), "title": "Call back",
	})
	expect(t, task, http.StatusCreated)
	if n := len(c.Do(http.MethodGet, "/retail/tasks?owner=me&status=open", nil).Items()); n != 1 {
		t.Fatalf("my open tasks: %d", n)
	}
	expect(t, c.Do(http.MethodPost, "/retail/tasks/"+str(task.Data(), "id")+"/complete", map[string]any{}, ifMatch("1")...), http.StatusOK)

	expect(t, c.Do(http.MethodPost, "/retail/listings", map[string]any{"vehicleId": "00000000-0000-4000-8000-00000000abcd", "text": "x", "askingPrice": usd("100")}), http.StatusUnprocessableEntity)
	li := c.Do(http.MethodPost, "/retail/listings", map[string]any{"vehicleId": s.vehicles[0], "text": "Cobalt LTZ 2025", "askingPrice": usd("1650000")})
	expect(t, li, http.StatusCreated)
	expect(t, c.Do(http.MethodPost, "/retail/listings", map[string]any{"vehicleId": s.vehicles[0], "text": "dup", "askingPrice": usd("1")}), http.StatusConflict, "listing_exists")
	expect(t, c.Do(http.MethodPost, "/retail/listings/"+str(li.Data(), "id")+"/publish", map[string]any{}, ifMatch("1")...), http.StatusOK)
}

func TestCashSaleToDelivery(t *testing.T) {
	s := newShop(t)
	c := s.c
	cust := s.customer(t, "Dilnoza")
	l := c.Do(http.MethodPost, "/retail/leads", map[string]any{"customerId": cust, "branchId": s.branch, "source": "website"})
	lead := str(l.Data(), "id")
	li := c.Do(http.MethodPost, "/retail/listings", map[string]any{"vehicleId": s.vehicles[0], "text": "Cobalt", "askingPrice": usd("1650000")})
	expect(t, c.Do(http.MethodPost, "/retail/listings/"+str(li.Data(), "id")+"/publish", map[string]any{}, ifMatch("1")...), http.StatusOK)

	deal := func(vehicle string, leadID any, scheme string) e2e.Response {
		return c.Do(http.MethodPost, "/retail/deals", map[string]any{
			"customerId": cust, "leadId": leadID, "vehicleId": vehicle,
			"branchId": s.branch, "paymentScheme": scheme, "price": usd("1600000"),
		})
	}
	expect(t, deal(s.vehicles[0], lead, "cash"), http.StatusConflict, "lead_not_eligible") // lead not qualified yet
	for i, st := range []string{"contacted", "qualified"} {
		expect(t, c.Do(http.MethodPost, "/retail/leads/"+lead+"/stage", map[string]string{"stage": st}, ifMatch([]string{"1", "2"}[i])...), http.StatusOK)
	}
	d := deal(s.vehicles[0], lead, "cash")
	expect(t, d, http.StatusCreated)
	id := str(d.Data(), "id")
	expect(t, deal(s.vehicles[0], nil, "cash"), http.StatusConflict) // one active sale per VIN
	expect(t, c.Do(http.MethodPost, "/retail/leads/"+lead+"/stage", map[string]string{"stage": "lost", "reason": "x"}, ifMatch("4")...), http.StatusConflict, "lead_in_sale")

	rev := func() string { return c.Do(http.MethodGet, "/retail/deals/"+id, nil).Revision() }
	now := s.e.Clock.Now().Format(time.RFC3339)
	c.EnrollMFA() // payment acceptance and delivery are sensitive
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+id+"/deliveries", map[string]string{"occurredAt": now}, ifMatch(rev())...), http.StatusConflict, "delivery_not_ready")
	record := func(files ...string) e2e.Response {
		return c.Do(http.MethodPost, "/retail/deals/"+id+"/contract-records", map[string]any{"signedOn": "2026-09-22", "reference": "DKP-17", "bindingIds": files}, ifMatch(rev())...)
	}
	expect(t, record("00000000-0000-4000-8000-00000000abcd"), http.StatusUnprocessableEntity) // not the company's file
	scan := str(c.Upload("deal-document", "contract.pdf", e2e.PDF).Data(), "id")
	signed := record(scan)
	expect(t, signed, http.StatusOK)
	if f := signed.Data()["contractFileIds"].([]any); len(f) != 1 || f[0] != scan {
		t.Fatalf("contract files: %v", signed.Data()["contractFileIds"])
	}
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+id+"/invoices", map[string]any{"purpose": "first-installment", "amount": usd("1"), "recipientSnapshot": "x"}, ifMatch(rev())...), http.StatusUnprocessableEntity)

	payInvoice := func(purpose string, amount map[string]string) {
		inv := c.Do(http.MethodPost, "/retail/deals/"+id+"/invoices", map[string]any{"purpose": purpose, "amount": amount, "recipientSnapshot": "Retail Motors LLC"}, ifMatch(rev())...)
		expect(t, inv, http.StatusCreated)
		ev := c.Do(http.MethodPost, "/retail/invoices/"+str(inv.Data(), "id")+"/evidence", map[string]any{"claimedAmount": amount, "paidOn": "2026-09-22", "externalReference": "CHK-" + purpose})
		expect(t, ev, http.StatusCreated)
		e := ev.Data()["paymentEvidence"].([]any)[0].(map[string]any)
		expect(t, c.Do(http.MethodPost, "/retail/evidence/"+str(e, "id")+"/accept", map[string]any{"confirmation": true}, ifMatch("1")...), http.StatusOK)
	}
	payInvoice("vehicle-payment", usd("1600000"))
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+id+"/registration", map[string]string{"registeredOn": "2026-09-22", "plateNumber": "01A123BC"}, ifMatch(rev())...), http.StatusConflict, "registration_unpaid")
	payInvoice("registration", map[string]string{"amountMinor": "150000000", "currency": "UZS"})
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+id+"/registration", map[string]string{"registeredOn": "2026-09-22", "plateNumber": "01A123BC"}, ifMatch(rev())...), http.StatusOK)

	del := c.Do(http.MethodPost, "/retail/deals/"+id+"/deliveries", map[string]string{"occurredAt": now}, ifMatch(rev())...)
	expect(t, del, http.StatusOK)
	if del.Data()["status"] != "delivered" {
		t.Fatalf("deal: %v", del.Data())
	}
	if c.Do(http.MethodGet, "/retail/leads/"+lead, nil).Data()["stage"] != "won" {
		t.Fatal("delivery must win the lead")
	}
	if c.Do(http.MethodGet, "/retail/listings/"+str(li.Data(), "id"), nil).Data()["status"] != "withdrawn" {
		t.Fatal("delivery must withdraw the listing")
	}
	expect(t, c.Do(http.MethodGet, "/inventory/vehicle-units/"+s.vehicles[0], nil), http.StatusNotFound) // left the company's stock
}

func TestSchemeGatesAndCancellation(t *testing.T) {
	s := newShop(t)
	c := s.c
	cust := s.customer(t, "Rustam")
	c.EnrollMFA()
	create := func(vehicle, scheme string) string {
		d := c.Do(http.MethodPost, "/retail/deals", map[string]any{
			"customerId": cust, "vehicleId": vehicle, "branchId": s.branch,
			"paymentScheme": scheme, "price": usd("1600000"),
		})
		expect(t, d, http.StatusCreated)
		return str(d.Data(), "id")
	}
	now := s.e.Clock.Now().Format(time.RFC3339)

	pf := create(s.vehicles[1], "partner-finance")
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+pf+"/deliveries", map[string]string{"occurredAt": now}, ifMatch("1")...), http.StatusForbidden, "policy_unresolved")

	oi := create(s.vehicles[2], "own-installment")
	d := c.Do(http.MethodGet, "/retail/deals/"+oi, nil)
	if d.Data()["checklist"].(map[string]any)["insuranceApproved"] != false {
		t.Fatalf("own-installment checklist: %v", d.Data()["checklist"])
	}

	// Cancelling releases the vehicle for a new sale.
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+pf+"/cancel", map[string]string{"reason": ""}, ifMatch("1")...), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, "/retail/deals/"+pf+"/cancel", map[string]string{"reason": "customer changed mind"}, ifMatch("1")...), http.StatusOK)
	create(s.vehicles[1], "cash")
}
