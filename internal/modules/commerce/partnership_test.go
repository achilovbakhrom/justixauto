package commerce_test

import (
	"net/http"
	"testing"

	"justixauto/internal/modules/commerce"
	"justixauto/internal/testkit"
)

var (
	expect  = testkit.Expect
	ifMatch = testkit.IfMatch
)

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func actions(r testkit.Response) []any { return r.Data()["allowedActions"].([]any) }

func TestPartnershipLifecycle(t *testing.T) {
	e := testkit.New(t)
	admin := e.Admin()
	perms := []string{commerce.PermRead, commerce.PermPartnershipsManage}
	a := e.CompanyUser(admin, "Alpha Motors", perms...)
	b := e.CompanyUser(admin, "Beta Motors", perms...)
	c := e.CompanyUser(admin, "Gamma Motors", perms...)
	reader := e.CompanyUser(admin, "Reader Motors", commerce.PermRead)

	// Seller directory shows active sellers' public profiles only.
	dir := a.Do(http.MethodGet, "/identity/directory/companies?kind=seller&q=motors", nil)
	expect(t, dir, http.StatusOK)
	if len(dir.Items()) != 4 {
		t.Fatalf("directory: %v", dir.Items())
	}
	if _, leaked := dir.Items()[0].(map[string]any)["email"]; leaked {
		t.Fatal("directory must not expose contacts")
	}

	req := a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": b.CompanyID})
	expect(t, req, http.StatusCreated)
	id := str(req.Data(), "id")
	if req.Data()["direction"] != "outgoing" || req.Data()["status"] != "requested" || len(actions(req)) != 1 {
		t.Fatalf("request: %v", req.Data())
	}
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": b.CompanyID}), http.StatusConflict, "partnership_exists")
	expect(t, b.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": a.CompanyID}), http.StatusConflict, "partnership_exists")
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": a.CompanyID}), http.StatusUnprocessableEntity)
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": "00000000-0000-4000-8000-00000000abcd"}), http.StatusUnprocessableEntity)
	expect(t, reader.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": c.CompanyID}), http.StatusForbidden, "permission_denied")

	// Only the invited company decides; the requester only withdraws.
	incoming := b.Do(http.MethodGet, "/commerce/partnerships?status=requested", nil).Items()
	if len(incoming) != 1 || incoming[0].(map[string]any)["direction"] != "incoming" {
		t.Fatalf("incoming: %v", incoming)
	}
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships/"+id+"/accept", map[string]any{}, ifMatch("1")...), http.StatusForbidden, "wrong_party")
	expect(t, b.Do(http.MethodPost, "/commerce/partnerships/"+id+"/decline", map[string]any{}, ifMatch("1")...), http.StatusUnprocessableEntity)
	acc := b.Do(http.MethodPost, "/commerce/partnerships/"+id+"/accept", map[string]any{}, ifMatch("1")...)
	expect(t, acc, http.StatusOK)
	if acc.Data()["status"] != "active" || acc.Data()["activatedAt"] == nil {
		t.Fatalf("accepted: %v", acc.Data())
	}
	expect(t, b.Do(http.MethodPost, "/commerce/partnerships/"+id+"/end", map[string]string{"reason": "x"}, ifMatch("1")...), http.StatusPreconditionFailed)
	expect(t, c.Do(http.MethodGet, "/commerce/partnerships/"+id, nil), http.StatusNotFound)

	// Either side can end it; afterwards a new request is possible.
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships/"+id+"/end", map[string]string{"reason": "contract finished"}, ifMatch("2")...), http.StatusOK)
	expect(t, b.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": a.CompanyID}), http.StatusCreated)

	// Withdrawn requests are closed for good.
	w := a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": c.CompanyID})
	expect(t, w, http.StatusCreated)
	wid := str(w.Data(), "id")
	expect(t, c.Do(http.MethodPost, "/commerce/partnerships/"+wid+"/withdraw", map[string]string{"reason": "x"}, ifMatch("1")...), http.StatusForbidden, "wrong_party")
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships/"+wid+"/withdraw", map[string]string{"reason": "sent by mistake"}, ifMatch("1")...), http.StatusOK)
	expect(t, c.Do(http.MethodPost, "/commerce/partnerships/"+wid+"/accept", map[string]any{}, ifMatch("2")...), http.StatusConflict, "invalid_transition")
	expect(t, a.Do(http.MethodPost, "/commerce/partnerships/"+wid+"/pause", map[string]any{}, ifMatch("2")...), http.StatusNotFound)
}

func TestOnlyActiveSellersTrade(t *testing.T) {
	e := testkit.New(t)
	admin := e.Admin()
	a := e.CompanyUser(admin, "Alpha Motors", commerce.PermRead, commerce.PermPartnershipsManage)

	draft := admin.Do(http.MethodPost, "/identity/admin/seller-companies", map[string]any{
		"company":    map[string]any{"name": "Draft Motors", "country": map[string]string{"label": "Uzbekistan"}, "registration": "D-1", "email": "d@d.test"},
		"firstAdmin": map[string]string{"displayName": "D", "login": "draft", "email": "draft@d.test", "password": "draft-password-1", "passwordConfirmation": "draft-password-1"},
	})
	expect(t, draft, http.StatusCreated)
	bank := admin.Do(http.MethodPost, "/identity/admin/provider-companies", map[string]any{
		"kind":       "bank",
		"company":    map[string]any{"name": "Capital Bank", "country": map[string]string{"label": "Uzbekistan"}, "registration": "B-1", "email": "b@b.test"},
		"firstAdmin": map[string]string{"displayName": "B", "login": "banker", "email": "banker@b.test", "password": "bank-password-12", "passwordConfirmation": "bank-password-12"},
	})
	expect(t, bank, http.StatusCreated)
	for _, r := range []testkit.Response{draft, bank} {
		id := str(r.Data()["company"].(map[string]any), "id")
		expect(t, a.Do(http.MethodPost, "/commerce/partnerships", map[string]string{"counterpartyCompanyId": id}), http.StatusConflict, "company_not_trading")
	}
}
