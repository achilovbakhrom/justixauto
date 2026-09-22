package inventory

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/testkit"
)

var expect = testkit.Expect
var ifMatch = testkit.IfMatch

func newEnv(t *testing.T) (*testkit.Env, *testkit.Client) {
	e := testkit.New(t, Permissions, func(api *echo.Group, db *gorm.DB, now func() time.Time) {
		New(db, now).Register(api)
	})
	return e, e.Admin()
}

var allPerms = []string{PermRead, PermModelsEdit, PermWarehousesManage, PermReceiptsCreate, PermVehiclesMove}

func spec(variant string, year int) map[string]any {
	return map[string]any{"specification": map[string]any{"make": "Chevrolet", "model": "Cobalt", "variant": variant,
		"year": year, "bodyType": "sedan", "exteriorColor": "white", "interiorColor": "black",
		"powertrain": "petrol 1.5", "drivetrain": "FWD"}}
}

func warehouse(name string, capacity any) map[string]any {
	return map[string]any{"name": name, "country": map[string]string{"key": "UZ", "label": "Uzbekistan"},
		"city": "Tashkent", "address": "Yunusabad 1", "capacity": capacity}
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func TestModelCatalogue(t *testing.T) {
	e, admin := newEnv(t)
	editor := e.CompanyUser(admin, "Motors", allPerms...)
	viewer := e.CompanyUser(admin, "Viewer", PermRead)

	m := editor.Do(http.MethodPost, "/inventory/vehicle-models", spec("LTZ", 2025))
	expect(t, m, http.StatusCreated)
	id := str(m.Data(), "id")
	expect(t, editor.Do(http.MethodPost, "/inventory/vehicle-models", spec("ltz", 2026)), http.StatusConflict, "model_duplicate")
	expect(t, viewer.Do(http.MethodPost, "/inventory/vehicle-models", spec("LS", 2025)), http.StatusForbidden, "permission_denied")
	bad := spec("LS", 1800)
	expect(t, editor.Do(http.MethodPost, "/inventory/vehicle-models", bad), http.StatusUnprocessableEntity)

	// A new specification version keeps make/model/variant and becomes current.
	v2 := editor.Do(http.MethodPost, "/inventory/vehicle-models/"+id+"/specification-versions", spec("LTZ", 2026), ifMatch("1")...)
	expect(t, v2, http.StatusCreated)
	if s := v2.Data()["specification"].(map[string]any); s["version"] != "2" || s["year"] != float64(2026) {
		t.Fatalf("current spec: %v", s)
	}
	expect(t, editor.Do(http.MethodPost, "/inventory/vehicle-models/"+id+"/specification-versions", spec("LT", 2026), ifMatch("2")...), http.StatusUnprocessableEntity)
	expect(t, editor.Do(http.MethodPost, "/inventory/vehicle-models/"+id+"/specification-versions", spec("LTZ", 2027), ifMatch("1")...), http.StatusPreconditionFailed)

	list := viewer.Do(http.MethodGet, "/inventory/vehicle-models?q=cob", nil)
	expect(t, list, http.StatusOK)
	if len(list.Items()) != 1 {
		t.Fatalf("catalogue search: %v", list.Items())
	}
	if n := len(viewer.Do(http.MethodGet, "/inventory/vehicle-models/"+id, nil).Data()["versions"].([]any)); n != 2 {
		t.Fatalf("versions: %d", n)
	}
}

func TestReceiptIdentifyMoveAndCapacity(t *testing.T) {
	e, admin := newEnv(t)
	c := e.CompanyUser(admin, "Motors", allPerms...)
	model := str(c.Do(http.MethodPost, "/inventory/vehicle-models", spec("LTZ", 2025)).Data(), "id")

	w1 := c.Do(http.MethodPost, "/inventory/warehouses", warehouse("Main yard", "3"))
	expect(t, w1, http.StatusCreated)
	w2 := c.Do(http.MethodPost, "/inventory/warehouses", warehouse("Branch yard", 1))
	expect(t, w2, http.StatusCreated)
	expect(t, c.Do(http.MethodPost, "/inventory/warehouses", warehouse("main YARD", 5)), http.StatusConflict, "warehouse_duplicate")
	expect(t, c.Do(http.MethodPost, "/inventory/warehouses", warehouse("Zero", 0)), http.StatusUnprocessableEntity)
	id1, id2 := str(w1.Data(), "id"), str(w2.Data(), "id")

	receipt := func(stock map[string]any) map[string]any {
		return map[string]any{"modelId": model, "modelSpecificationVersion": "1", "stock": stock,
			"receivedAt": e.Clock.Now().Format(time.RFC3339)}
	}
	path := "/inventory/warehouses/" + id1 + "/receipt-batches"

	// Known VINs: vehicles, placements and batch in one step.
	r := c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "identified", "vins": []string{" xtaaa11111a000001 "}}), ifMatch("1")...)
	expect(t, r, http.StatusCreated)
	if w := r.Data()["warehouse"].(map[string]any); w["occupied"] != "1" || w["free"] != "2" || w["revision"] != "2" {
		t.Fatalf("after identified receipt: %v", w)
	}
	vehicleID := str(r.Data()["vehicles"].([]any)[0].(map[string]any), "id")

	// A VIN exists once, anywhere; the whole receipt is rejected.
	expect(t, c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A000002", "XTAAA11111A000001"}}), ifMatch("2")...), http.StatusConflict, "VIN_UNAVAILABLE")
	expect(t, c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A00000I"}}), ifMatch("2")...), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A000002", "xtaaa11111a000002"}}), ifMatch("2")...), http.StatusUnprocessableEntity)
	expect(t, c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "unidentified", "quantity": "1"}), ifMatch("1")...), http.StatusPreconditionFailed)

	// Unidentified stock occupies space without fake vehicles; capacity is enforced.
	expect(t, c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "unidentified", "quantity": "3"}), ifMatch("2")...), http.StatusConflict, "capacity_exceeded")
	b := c.Do(http.MethodPost, path, receipt(map[string]any{"mode": "unidentified", "quantity": "2"}), ifMatch("2")...)
	expect(t, b, http.StatusCreated)
	batchID := str(b.Data()["batch"].(map[string]any), "id")
	stock := c.Do(http.MethodGet, "/inventory/warehouses/"+id1+"/inventory", nil)
	if len(stock.Data()["vehicles"].([]any)) != 1 || len(stock.Data()["unidentifiedBatches"].([]any)) != 1 ||
		stock.Data()["warehouse"].(map[string]any)["free"] != "0" {
		t.Fatalf("stock: %v", stock.Data())
	}
	expect(t, c.Do(http.MethodPost, "/inventory/warehouses/"+id1+"/capacity-changes", map[string]any{"capacity": "2", "reason": "smaller lot"}, ifMatch("3")...), http.StatusConflict, "capacity_below_occupied")

	// Entering VINs later: occupancy unchanged, atomic, model must match.
	ident := func(vins ...string) map[string]any {
		items := []map[string]string{}
		for _, v := range vins {
			items = append(items, map[string]string{"vin": v, "modelId": model})
		}
		return map[string]any{"items": items, "atomic": true}
	}
	idPath := "/inventory/receipt-batches/" + batchID + "/identifications"
	expect(t, c.Do(http.MethodPost, idPath, ident("XTAAA11111A000003", "XTAAA11111A000004", "XTAAA11111A000005")), http.StatusConflict, "exceeds_unidentified")
	expect(t, c.Do(http.MethodPost, idPath, ident("XTAAA11111A000003", "XTAAA11111A000001")), http.StatusConflict, "VIN_UNAVAILABLE")
	idr := c.Do(http.MethodPost, idPath, ident("XTAAA11111A000003"))
	expect(t, idr, http.StatusOK)
	if bt := idr.Data()["batch"].(map[string]any); bt["identifiedCount"] != "1" || bt["unidentifiedCount"] != "1" {
		t.Fatalf("batch after identification: %v", bt)
	}
	if w := idr.Data()["warehouse"].(map[string]any); w["occupied"] != "3" {
		t.Fatalf("occupancy must not change: %v", w)
	}

	// Moves: same company, destination capacity, stale source detected.
	move := func(from, to string) map[string]any {
		return map[string]any{"fromWarehouseId": from, "toWarehouseId": to, "occurredAt": e.Clock.Now().Format(time.RFC3339)}
	}
	mv := c.Do(http.MethodPost, "/inventory/vehicle-units/"+vehicleID+"/warehouse-moves", move(id1, id2))
	expect(t, mv, http.StatusOK)
	if p := mv.Data()["placement"].(map[string]any); p["warehouseId"] != id2 {
		t.Fatalf("placement after move: %v", p)
	}
	expect(t, c.Do(http.MethodPost, "/inventory/vehicle-units/"+vehicleID+"/warehouse-moves", move(id1, id2)), http.StatusConflict, "placement_changed")
	other := str(idr.Data()["vehicles"].([]any)[0].(map[string]any), "id")
	expect(t, c.Do(http.MethodPost, "/inventory/vehicle-units/"+other+"/warehouse-moves", move(id1, id2)), http.StatusConflict, "capacity_exceeded")

	// Vehicle detail carries its specification and full history.
	d := c.Do(http.MethodGet, "/inventory/vehicle-units/"+vehicleID, nil)
	expect(t, d, http.StatusOK)
	var types []string
	for _, h := range d.Data()["history"].([]any) {
		types = append(types, h.(map[string]any)["type"].(string))
	}
	if len(types) != 2 || types[0] != "vehicle.received" || types[1] != "vehicle.moved" {
		t.Fatalf("history: %v", types)
	}
	if n := len(c.Do(http.MethodGet, "/inventory/vehicle-units?placement=warehouse&warehouseId="+id1, nil).Items()); n != 1 {
		t.Fatalf("vehicles in main yard: %d", n)
	}
	expect(t, c.Do(http.MethodGet, "/inventory/vehicle-units?placement=nowhere", nil), http.StatusUnprocessableEntity)
}

func TestTenantIsolation(t *testing.T) {
	e, admin := newEnv(t)
	a := e.CompanyUser(admin, "Alpha", allPerms...)
	b := e.CompanyUser(admin, "Beta", allPerms...)
	model := str(a.Do(http.MethodPost, "/inventory/vehicle-models", spec("LTZ", 2025)).Data(), "id")
	wa := str(a.Do(http.MethodPost, "/inventory/warehouses", warehouse("Alpha yard", 5)).Data(), "id")
	wb := str(b.Do(http.MethodPost, "/inventory/warehouses", warehouse("Beta yard", 5)).Data(), "id")
	r := a.Do(http.MethodPost, "/inventory/warehouses/"+wa+"/receipt-batches", map[string]any{"modelId": model,
		"modelSpecificationVersion": "1", "stock": map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A000009"}},
		"receivedAt": e.Clock.Now().Format(time.RFC3339)}, ifMatch("1")...)
	expect(t, r, http.StatusCreated)
	vehicle := str(r.Data()["vehicles"].([]any)[0].(map[string]any), "id")

	// Other companies see nothing: not the warehouse, the vehicle or its VIN owner.
	expect(t, b.Do(http.MethodGet, "/inventory/warehouses/"+wa, nil), http.StatusNotFound)
	expect(t, b.Do(http.MethodGet, "/inventory/vehicle-units/"+vehicle, nil), http.StatusNotFound)
	if n := len(b.Do(http.MethodGet, "/inventory/vehicle-units", nil).Items()); n != 0 {
		t.Fatalf("beta sees %d vehicles", n)
	}
	dup := b.Do(http.MethodPost, "/inventory/warehouses/"+wb+"/receipt-batches", map[string]any{"modelId": model,
		"modelSpecificationVersion": "1", "stock": map[string]any{"mode": "identified", "vins": []string{"XTAAA11111A000009"}},
		"receivedAt": e.Clock.Now().Format(time.RFC3339)}, ifMatch("1")...)
	expect(t, dup, http.StatusConflict, "VIN_UNAVAILABLE")
	// Moving into another company's warehouse is refused.
	expect(t, a.Do(http.MethodPost, "/inventory/vehicle-units/"+vehicle+"/warehouse-moves", map[string]any{
		"fromWarehouseId": wa, "toWarehouseId": wb, "occurredAt": e.Clock.Now().Format(time.RFC3339)}), http.StatusUnprocessableEntity)

	// Company routes need an active company.
	fresh := e.Browser()
	s := fresh.SignIn("alpha", "company-password-1")
	expect(t, s, http.StatusOK)
	expect(t, fresh.Do(http.MethodGet, "/inventory/warehouses", nil), http.StatusConflict, "no_active_company")
	expect(t, fresh.Do(http.MethodGet, "/inventory/warehouses", nil, "X-Context-Revision", "99"), http.StatusConflict)
}

// Concurrent moves into a warehouse with one free place: exactly one wins.
func TestConcurrentMovesRespectCapacity(t *testing.T) {
	e, admin := newEnv(t)
	c := e.CompanyUser(admin, "Motors", allPerms...)
	model := str(c.Do(http.MethodPost, "/inventory/vehicle-models", spec("LTZ", 2025)).Data(), "id")
	from := str(c.Do(http.MethodPost, "/inventory/warehouses", warehouse("Big", 10)).Data(), "id")
	to := str(c.Do(http.MethodPost, "/inventory/warehouses", warehouse("Small", 1)).Data(), "id")
	vins := []string{"XTAAA11111A000011", "XTAAA11111A000012", "XTAAA11111A000013", "XTAAA11111A000014", "XTAAA11111A000015"}
	r := c.Do(http.MethodPost, "/inventory/warehouses/"+from+"/receipt-batches", map[string]any{"modelId": model,
		"modelSpecificationVersion": "1", "stock": map[string]any{"mode": "identified", "vins": vins},
		"receivedAt": e.Clock.Now().Format(time.RFC3339)}, ifMatch("1")...)
	expect(t, r, http.StatusCreated)

	statuses := make(chan int, len(vins))
	var wg sync.WaitGroup
	for _, v := range r.Data()["vehicles"].([]any) {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			statuses <- c.Do(http.MethodPost, "/inventory/vehicle-units/"+id+"/warehouse-moves", map[string]any{
				"fromWarehouseId": from, "toWarehouseId": to, "occurredAt": e.Clock.Now().Format(time.RFC3339)}).Status
		}(str(v.(map[string]any), "id"))
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for s := range statuses {
		counts[s]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != len(vins)-1 {
		t.Fatalf("move outcomes: %v", counts)
	}
	if w := c.Do(http.MethodGet, "/inventory/warehouses/"+to, nil).Data(); w["occupied"] != "1" {
		t.Fatalf("small warehouse: %v", w)
	}
}
