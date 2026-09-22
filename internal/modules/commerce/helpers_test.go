package commerce_test

import (
	"net/http"
	"testing"

	"justixauto/internal/modules/commerce"
	"justixauto/internal/modules/inventory"
	"justixauto/internal/testkit"
)

// trade is a supplier and a buyer in an active partnership, with one model.
type trade struct {
	e               *testkit.Env
	admin           *testkit.Client
	supplier, buyer *testkit.Client
	model           string
	partnership     string
}

var tradePerms = []string{commerce.PermRead, commerce.PermPartnershipsManage, commerce.PermOffersManage, commerce.PermTrade, commerce.PermPaymentsAccept,
	inventory.PermRead, inventory.PermModelsEdit, "documents.read", "documents.upload", inventory.PermWarehousesManage, inventory.PermReceiptsCreate, inventory.PermVehiclesMove}

func newTrade(t *testing.T) *trade {
	e := testkit.New(t)
	admin := e.Admin()
	tr := &trade{e: e, admin: admin, supplier: e.CompanyUser(admin, "Supplier Motors", tradePerms...),
		buyer: e.CompanyUser(admin, "Buyer Motors", tradePerms...)}
	tr.partnership = partner(t, tr.supplier, tr.buyer)
	tr.model = str(tr.supplier.Do(http.MethodPost, "/inventory/vehicle-models", map[string]any{"specification": map[string]any{
		"make": "Chevrolet", "model": "Onix", "variant": "LTZ", "year": 2025, "bodyType": "sedan",
		"exteriorColor": "white", "interiorColor": "black", "powertrain": "petrol 1.2T", "drivetrain": "FWD"}}).Data(), "id")
	return tr
}

// publishedOffer publishes an all-active offer with two lines (2 × 1500000 + 1 × 1600000 USD).
func (tr *trade) publishedOffer(t *testing.T) (offerID string, version map[string]any) {
	created := tr.supplier.Do(http.MethodPost, "/commerce/offers", map[string]any{"terms": terms(tr.model,
		map[string]any{"amount": usd("4600000"), "dueDate": "2026-10-01"}), "audience": map[string]any{"mode": "all-active"}})
	expect(t, created, http.StatusCreated)
	offerID = str(created.Data(), "id")
	v := created.Data()["versions"].([]any)[0].(map[string]any)
	pub := tr.supplier.Do(http.MethodPost, "/commerce/offers/"+offerID+"/publish", map[string]string{"offerVersionId": str(v, "id")}, ifMatch("1")...)
	expect(t, pub, http.StatusOK)
	return offerID, pub.Data()["publishedVersion"].(map[string]any)
}
