package testkit

import (
	"net/http"
	"time"
)

// Shop is a seller company ready to sell: a branch, a model, a warehouse with
// vehicles and a customer. Its user holds every retail and inventory
// permission plus extra.
type Shop struct {
	*Client
	Branch, Customer string
	Vehicles         []string
}

var shopPerms = []string{"retail.read", "retail.crm.manage", "retail.listings.manage", "retail.deals.manage",
	"retail.payments.accept", "retail.deals.deliver", "inventory.read", "inventory.models.edit",
	"inventory.warehouses.manage", "inventory.receipts.create"}

// NewShop creates the shop with n vehicles (VINs from prefix + index).
func (e *Env) NewShop(admin *Client, name, vinPrefix string, n int, extra ...string) *Shop {
	t := e.T
	t.Helper()
	c := e.CompanyUser(admin, name, append(shopPerms, extra...)...)
	br := c.Do(http.MethodPost, "/identity/companies/"+c.CompanyID+"/branches", map[string]any{"name": "Main"})
	Expect(t, br, http.StatusCreated)
	model := c.Do(http.MethodPost, "/inventory/vehicle-models", map[string]any{"specification": map[string]any{"make": "Chevrolet",
		"model": "Cobalt " + name, "variant": "LTZ", "year": 2025, "bodyType": "sedan", "exteriorColor": "white", "interiorColor": "black",
		"powertrain": "petrol", "drivetrain": "FWD"}})
	Expect(t, model, http.StatusCreated)
	w := c.Do(http.MethodPost, "/inventory/warehouses", map[string]any{"name": "Showroom", "country": map[string]string{"label": "Uzbekistan"},
		"city": "Tashkent", "address": "1", "capacity": "50"})
	Expect(t, w, http.StatusCreated)
	var vins []string
	for i := 0; i < n; i++ {
		vins = append(vins, vinPrefix+string(rune('A'+i)))
	}
	r := c.Do(http.MethodPost, "/inventory/warehouses/"+w.Data()["id"].(string)+"/receipt-batches", map[string]any{
		"modelId": model.Data()["id"], "modelSpecificationVersion": "1", "stock": map[string]any{"mode": "identified", "vins": vins},
		"receivedAt": e.Clock.Now().Format(time.RFC3339)}, IfMatch("1")...)
	Expect(t, r, http.StatusCreated)
	cu := c.Do(http.MethodPost, "/retail/customers", map[string]any{"profile": map[string]string{"displayName": "Customer of " + name}})
	Expect(t, cu, http.StatusCreated)
	s := &Shop{Client: c, Branch: br.Data()["id"].(string), Customer: cu.Data()["id"].(string)}
	for _, v := range r.Data()["vehicles"].([]any) {
		s.Vehicles = append(s.Vehicles, v.(map[string]any)["id"].(string))
	}
	return s
}

// Sale creates a sale of vehicle i with the given payment scheme (USD price).
func (s *Shop) Sale(i int, scheme, priceMinor string) Response {
	s.env.T.Helper()
	d := s.Do(http.MethodPost, "/retail/deals", map[string]any{"customerId": s.Customer, "vehicleId": s.Vehicles[i],
		"branchId": s.Branch, "paymentScheme": scheme, "price": map[string]string{"amountMinor": priceMinor, "currency": "USD"}})
	Expect(s.env.T, d, http.StatusCreated)
	return d
}
