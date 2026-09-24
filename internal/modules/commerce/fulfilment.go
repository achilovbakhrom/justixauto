package commerce

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/database"
	"justixauto/internal/pkg/validate"
)

// Stock reserves and hands over vehicles (implemented by inventory). Calls
// made with a ctx from Store.Bind join the commerce transaction.
type Stock interface {
	Vehicle(ctx context.Context, companyID, id string) (*StockVehicle, error)
	Reserve(ctx context.Context, companyID, orderID string, vehicleIDs []string) error
	Release(ctx context.Context, orderID string, vehicleIDs []string, reason string) error
	Transfer(ctx context.Context, orderID string, vehicleIDs []string, toCompanyID, toWarehouseID, actorID string, at time.Time) error
}

type StockVehicle struct{ ID, VIN, ModelID string }

// ---- model ----

type Allocation struct {
	ID         string  `gorm:"primaryKey;type:uuid"`
	OrderID    string  `gorm:"type:uuid"`
	LineID     string  `gorm:"type:uuid"`
	VehicleID  string  `gorm:"type:uuid"`
	VIN        string  `gorm:"column:vin"`
	Status     string  // allocated | shipped | delivered | rejected | released
	ShipmentID *string `gorm:"type:uuid"`
	CreatedAt  time.Time
}

func (Allocation) TableName() string { return "commerce.order_allocations" }

// counts reports whether an allocation still counts toward its line.
func (a Allocation) counts() bool {
	return a.Status == "allocated" || a.Status == "shipped" || a.Status == "delivered"
}

type Shipment struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	OrderID   string `gorm:"type:uuid"`
	Route     string
	Status    string // in-transit | received
	Version   int64
	CreatedBy string `gorm:"type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Shipment) TableName() string { return "commerce.shipments" }

type Milestone struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	ShipmentID    string `gorm:"type:uuid"`
	MilestoneType string
	OccurredAt    time.Time
	Location      string
	Note          string
	RecordedBy    string `gorm:"type:uuid"`
	CompanyID     string `gorm:"type:uuid"`
	RecordedAt    time.Time
}

func (Milestone) TableName() string { return "commerce.shipment_milestones" }

var milestoneTypes = []string{"departed", "border-crossed", "customs-cleared", "arrived", "damage-reported"}

// ---- repository ----

type FulfilmentRepository interface {
	Allocations(ctx context.Context, orderID string) ([]Allocation, error)
	AddAllocations(ctx context.Context, as []Allocation) error
	SetAllocationStatus(ctx context.Context, orderID string, vehicleIDs []string, status string, shipmentID *string) error
	CreateShipment(ctx context.Context, s *Shipment) error
	Shipment(ctx context.Context, id string) (*Shipment, error)
	Shipments(ctx context.Context, orderID string) ([]Shipment, error)
	UpdateShipment(ctx context.Context, s *Shipment, expected int64) error
	AddMilestone(ctx context.Context, m *Milestone) error
	Milestones(ctx context.Context, shipmentID string) ([]Milestone, error)
}

type fulfilmentRepository struct{ db *gorm.DB }

func (r *fulfilmentRepository) Allocations(ctx context.Context, orderID string) ([]Allocation, error) {
	as := []Allocation{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at, vin").Find(&as).Error
	return as, database.Translate(err)
}

func (r *fulfilmentRepository) AddAllocations(ctx context.Context, as []Allocation) error {
	return database.Translate(r.db.WithContext(ctx).Create(&as).Error)
}

func (r *fulfilmentRepository) SetAllocationStatus(ctx context.Context, orderID string, vehicleIDs []string, status string, shipmentID *string) error {
	fields := map[string]any{"status": status}
	if shipmentID != nil {
		fields["shipment_id"] = *shipmentID
	}
	q := r.db.WithContext(ctx).Model(&Allocation{}).Where("order_id = ? AND status IN ('allocated', 'shipped', 'delivered')", orderID)
	if vehicleIDs != nil {
		q = q.Where("vehicle_id IN ?", vehicleIDs)
	}
	return database.Translate(q.Updates(fields).Error)
}

func (r *fulfilmentRepository) CreateShipment(ctx context.Context, s *Shipment) error {
	return database.Translate(r.db.WithContext(ctx).Create(s).Error)
}

func (r *fulfilmentRepository) Shipment(ctx context.Context, id string) (*Shipment, error) {
	var s Shipment
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&s).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &s, nil
}

func (r *fulfilmentRepository) Shipments(ctx context.Context, orderID string) ([]Shipment, error) {
	ss := []Shipment{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at, id").Find(&ss).Error
	return ss, database.Translate(err)
}

func (r *fulfilmentRepository) UpdateShipment(ctx context.Context, s *Shipment, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Shipment{}, s.ID, expected, map[string]any{"status": s.Status, "updated_at": s.UpdatedAt})
	if err == nil {
		s.Version = expected + 1
	}
	return err
}

func (r *fulfilmentRepository) AddMilestone(ctx context.Context, m *Milestone) error {
	return database.Translate(r.db.WithContext(ctx).Create(m).Error)
}

func (r *fulfilmentRepository) Milestones(ctx context.Context, shipmentID string) ([]Milestone, error) {
	ms := []Milestone{}
	err := r.db.WithContext(ctx).Where("shipment_id = ?", shipmentID).Order("occurred_at, recorded_at").Find(&ms).Error
	return ms, database.Translate(err)
}

// ---- service ----

type FulfilmentService struct{ deps }

// AllocationItem assigns one concrete vehicle to an order line.
type AllocationItem struct {
	OrderLineID string `json:"orderLineId"`
	VehicleID   string `json:"vehicleId"`
}

// Allocate reserves concrete vehicles of the supplier for order lines. Each
// vehicle must match the line's model; a line never gets more vehicles than
// its quantity; a vehicle can be held by only one deal at a time.
func (s *FulfilmentService) Allocate(ctx context.Context, p *auth.Principal, orderID string, expected int64, items []AllocationItem) (*Order, error) {
	if len(items) == 0 || len(items) > 1000 {
		return nil, apperr.FieldError("items", "list 1-1000 vehicles")
	}
	var o *Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.supplierOrder(ctx, st, p, orderID, expected); err != nil {
			return err
		}
		if o.Status != OrderAccepted && o.Status != OrderFulfilling {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "vehicles are allocated to accepted orders")
		}
		existing, err := st.Fulfilment().Allocations(ctx, o.ID)
		if err != nil {
			return err
		}
		now := s.clock()
		add, vehicleIDs, err := s.planAllocations(ctx, st, p, o, existing, items, now)
		if err != nil {
			return err
		}
		if err := s.stock.Reserve(st.Bind(ctx), p.CompanyID, o.ID, vehicleIDs); err != nil {
			return err
		}
		if err := st.Fulfilment().AddAllocations(ctx, add); err != nil {
			return err
		}
		o.UpdatedAt = now
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.vehicles_allocated", "order", o.ID, "", map[string]any{"vehicleIds": vehicleIDs})
	})
	return o, err
}

// planAllocations validates each requested item against the order's lines,
// already-used quantities and the caller's stock, returning the allocations
// to insert and the vehicle IDs to reserve. It preserves the original
// per-item validation order and error precedence.
func (s *FulfilmentService) planAllocations(ctx context.Context, st Store, p *auth.Principal, o *Order, existing []Allocation, items []AllocationItem, now time.Time) ([]Allocation, []string, error) {
	used := map[string]int{}
	for _, a := range existing {
		if a.counts() {
			used[a.LineID]++
		}
	}
	lines := map[string]Line{}
	for _, l := range o.terms().Lines {
		lines[l.LineID] = l
	}
	var v apperr.Validation
	var add []Allocation
	var vehicleIDs []string
	for i, it := range items {
		field := "items." + strconv.Itoa(i)
		line, ok := lines[it.OrderLineID]
		if !ok {
			v.Add(field+".orderLineId", "not a line of this order")
			continue
		}
		if slices.Contains(vehicleIDs, it.VehicleID) || slices.ContainsFunc(existing, func(a Allocation) bool { return a.VehicleID == it.VehicleID && a.counts() }) {
			v.Add(field+".vehicleId", "already allocated")
			continue
		}
		vehicle, err := s.stock.Vehicle(st.Bind(ctx), p.CompanyID, it.VehicleID)
		if err != nil {
			v.Add(field+".vehicleId", "not one of your vehicles")
			continue
		}
		if vehicle.ModelID != line.ModelID {
			v.Add(field+".vehicleId", "VIN "+vehicle.VIN+" is a different model than the order line")
			continue
		}
		used[line.LineID]++
		if used[line.LineID] > int(line.Quantity) {
			v.Add(field+".orderLineId", "more vehicles than the ordered quantity")
		}
		vehicleIDs = append(vehicleIDs, it.VehicleID)
		add = append(add, Allocation{ID: uuid.NewString(), OrderID: o.ID, LineID: line.LineID, VehicleID: it.VehicleID, VIN: vehicle.VIN, Status: "allocated", CreatedAt: now})
	}
	if err := v.Err(); err != nil {
		return nil, nil, err
	}
	return add, vehicleIDs, nil
}

func (s *FulfilmentService) supplierOrder(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Order, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := st.Deals().Order(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if o.Version != expected {
		return nil, apperr.ErrStale
	}
	if o.Party(p.CompanyID) != "supplier" {
		return nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the supplier can do this")
	}
	return o, nil
}

type ShipmentInput struct {
	VehicleIDs []string `json:"vehicleIds"`
	Route      string   `json:"route"`
}

// Ship sends allocated vehicles; only concrete, allocated VINs can be shipped.
func (s *FulfilmentService) Ship(ctx context.Context, p *auth.Principal, orderID string, expected int64, in ShipmentInput) (*Shipment, error) {
	var v apperr.Validation
	ids := validate.UniqueIDs(&v, "vehicleIds", in.VehicleIDs)
	if len(ids) == 0 {
		v.Add("vehicleIds", "list the vehicles to ship")
	}
	if !slices.Contains(routes, in.Route) {
		v.Add("route", "must be factory, foreign-direct, in-transit or local")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var sh *Shipment
	err := s.store.InTx(ctx, func(st Store) error {
		o, err := s.supplierOrder(ctx, st, p, orderID, expected)
		if err != nil {
			return err
		}
		if o.Status != OrderAccepted && o.Status != OrderFulfilling {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "only accepted orders can be shipped")
		}
		allocs, err := st.Fulfilment().Allocations(ctx, o.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !slices.ContainsFunc(allocs, func(a Allocation) bool { return a.VehicleID == id && a.Status == "allocated" }) {
				return apperr.FieldError("vehicleIds", "only vehicles allocated to this order and not yet shipped")
			}
		}
		now := s.clock()
		sh = &Shipment{
			ID: uuid.NewString(), OrderID: o.ID, Route: in.Route, Status: "in-transit", Version: 1,
			CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.Fulfilment().CreateShipment(ctx, sh); err != nil {
			return err
		}
		if err := st.Fulfilment().SetAllocationStatus(ctx, o.ID, ids, "shipped", &sh.ID); err != nil {
			return err
		}
		o.Status, o.UpdatedAt = OrderFulfilling, now
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.shipped", "order", o.ID, "", map[string]any{"shipmentId": sh.ID, "vehicleIds": ids})
	})
	return sh, err
}

// shipment returns a shipment of an order the company takes part in.
func (s *FulfilmentService) shipment(ctx context.Context, st Store, p *auth.Principal, id string) (*Shipment, *Order, error) {
	if err := validate.IDs(id); err != nil {
		return nil, nil, err
	}
	sh, err := st.Fulfilment().Shipment(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	o, err := st.Deals().Order(ctx, p.CompanyID, sh.OrderID)
	if err != nil {
		return nil, nil, err
	}
	return sh, o, nil
}

type MilestoneInput struct {
	MilestoneType string    `json:"milestoneType"`
	OccurredAt    time.Time `json:"occurredAt"`
	Location      string    `json:"location"`
	Note          string    `json:"note"`
}

// AddMilestone records a route fact (who, when, where). It changes no state:
// receipt is a separate decision by the buyer.
func (s *FulfilmentService) AddMilestone(ctx context.Context, p *auth.Principal, shipmentID string, in MilestoneInput) (*ShipmentView, error) {
	var v apperr.Validation
	if !slices.Contains(milestoneTypes, in.MilestoneType) {
		v.Add("milestoneType", "unknown milestone")
	}
	if in.OccurredAt.IsZero() || in.OccurredAt.After(s.clock().Add(5*time.Minute)) {
		v.Add("occurredAt", "required and not in the future")
	}
	location := validate.Text(&v, "location", in.Location, 1, 300)
	note := validate.Text(&v, "note", in.Note, 0, 2000)
	if in.MilestoneType == "damage-reported" && note == "" {
		v.Add("note", "describe the damage")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		sh, _, err := s.shipment(ctx, st, p, shipmentID)
		if err != nil {
			return err
		}
		if sh.Status != "in-transit" && in.MilestoneType != "damage-reported" {
			return apperr.New(apperr.ErrConflict, "shipment_received", "the shipment was already received")
		}
		m := &Milestone{
			ID: uuid.NewString(), ShipmentID: sh.ID, MilestoneType: in.MilestoneType, OccurredAt: in.OccurredAt.UTC(),
			Location: location, Note: note, RecordedBy: p.UserID, CompanyID: p.CompanyID, RecordedAt: s.clock(),
		}
		if err := st.Fulfilment().AddMilestone(ctx, m); err != nil {
			return err
		}
		return s.event(ctx, st, p, "shipment."+in.MilestoneType, "order", sh.OrderID, note, map[string]any{"shipmentId": sh.ID, "location": location})
	})
	if err != nil {
		return nil, err
	}
	return s.GetShipment(ctx, p, shipmentID)
}

type ReceiptDecision struct {
	Decision    string   `json:"decision"` // accept | reject
	VehicleIDs  []string `json:"vehicleIds"`
	WarehouseID string   `json:"warehouseId"`
	Reason      string   `json:"reason"`
}

// DecideReceipt is the buyer's receipt of shipped vehicles. Accepted vehicles
// become the buyer's, placed in its warehouse (capacity checked). Rejected
// vehicles need a reason and go back to the supplier's free stock. When every
// line is delivered, the order is completed.
func (s *FulfilmentService) DecideReceipt(ctx context.Context, p *auth.Principal, shipmentID string, expected int64, in ReceiptDecision) (*ShipmentView, error) {
	var v apperr.Validation
	ids := validate.UniqueIDs(&v, "vehicleIds", in.VehicleIDs)
	if len(ids) == 0 {
		v.Add("vehicleIds", "list the vehicles")
	}
	switch in.Decision {
	case "accept":
		if uuid.Validate(in.WarehouseID) != nil {
			v.Add("warehouseId", "choose the receiving warehouse")
		}
	case "reject":
		in.Reason = validate.Reason(&v, in.Reason)
	default:
		v.Add("decision", "must be accept or reject")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		sh, o, err := s.shipment(ctx, st, p, shipmentID)
		if err != nil {
			return err
		}
		if o.Party(p.CompanyID) != "buyer" {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the buyer receives a shipment")
		}
		if sh.Version != expected {
			return apperr.ErrStale
		}
		allocs, err := st.Fulfilment().Allocations(ctx, o.ID)
		if err != nil {
			return err
		}
		inShipment := func(a Allocation) bool { return a.ShipmentID != nil && *a.ShipmentID == sh.ID && a.Status == "shipped" }
		if err := requireShipmentVehicles(allocs, ids, inShipment); err != nil {
			return err
		}
		now := s.clock()
		status, err := s.applyReceiptDecision(st.Bind(ctx), o.ID, ids, p, in, now)
		if err != nil {
			return err
		}
		if err := st.Fulfilment().SetAllocationStatus(ctx, o.ID, ids, status, nil); err != nil {
			return err
		}
		pending, delivered := receiptProgress(allocs, ids, inShipment, status)
		if pending == 0 {
			sh.Status, sh.UpdatedAt = "received", now
		}
		sh.UpdatedAt = now
		if err := st.Fulfilment().UpdateShipment(ctx, sh, expected); err != nil {
			return err
		}
		if orderComplete(o, delivered) {
			o.Status = OrderCompleted
		}
		o.UpdatedAt = now
		if err := st.Deals().UpdateOrder(ctx, o, o.Version); err != nil {
			return err
		}
		return s.event(ctx, st, p, "shipment.receipt_"+in.Decision+"ed", "order", o.ID, in.Reason,
			map[string]any{"shipmentId": sh.ID, "vehicleIds": ids, "warehouseId": in.WarehouseID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetShipment(ctx, p, shipmentID)
}

// requireShipmentVehicles checks that every id in ids is an allocation of
// this shipment awaiting receipt.
func requireShipmentVehicles(allocs []Allocation, ids []string, inShipment func(Allocation) bool) error {
	for _, id := range ids {
		if !slices.ContainsFunc(allocs, func(a Allocation) bool { return a.VehicleID == id && inShipment(a) }) {
			return apperr.FieldError("vehicleIds", "only vehicles of this shipment awaiting receipt")
		}
	}
	return nil
}

// applyReceiptDecision moves the vehicles into the buyer's stock (accept) or
// back to the supplier's free stock (reject), returning the resulting
// allocation status.
func (s *FulfilmentService) applyReceiptDecision(txctx context.Context, orderID string, ids []string, p *auth.Principal, in ReceiptDecision, now time.Time) (string, error) {
	if in.Decision == "accept" {
		if err := s.stock.Transfer(txctx, orderID, ids, p.CompanyID, in.WarehouseID, p.UserID, now); err != nil {
			return "", err
		}
		return "delivered", nil
	}
	if err := s.stock.Release(txctx, orderID, ids, "rejected at receipt: "+in.Reason); err != nil {
		return "", err
	}
	return "rejected", nil
}

// receiptProgress reports how many shipment vehicles still await a decision
// and how many vehicles are now delivered per order line. A shipment is
// received once nothing in it awaits a decision.
func receiptProgress(allocs []Allocation, ids []string, inShipment func(Allocation) bool, status string) (pending int, delivered map[string]int) {
	delivered = map[string]int{}
	for _, a := range allocs {
		decided := inShipment(a) && slices.Contains(ids, a.VehicleID)
		if inShipment(a) && !decided {
			pending++
		}
		if a.Status == "delivered" || (decided && status == "delivered") {
			delivered[a.LineID]++
		}
	}
	return pending, delivered
}

// orderComplete reports whether every line of the order has reached its
// ordered quantity of delivered vehicles.
func orderComplete(o *Order, delivered map[string]int) bool {
	for _, l := range o.terms().Lines {
		if delivered[l.LineID] < int(l.Quantity) {
			return false
		}
	}
	return true
}

type ShipmentView struct {
	Shipment   Shipment
	Milestones []Milestone
	Vehicles   []Allocation
}

func (s *FulfilmentService) GetShipment(ctx context.Context, p *auth.Principal, id string) (*ShipmentView, error) {
	sh, _, err := s.shipment(ctx, s.store, p, id)
	if err != nil {
		return nil, err
	}
	return s.shipmentView(ctx, sh)
}

func (s *FulfilmentService) shipmentView(ctx context.Context, sh *Shipment) (*ShipmentView, error) {
	ms, err := s.store.Fulfilment().Milestones(ctx, sh.ID)
	if err != nil {
		return nil, err
	}
	allocs, err := s.store.Fulfilment().Allocations(ctx, sh.OrderID)
	if err != nil {
		return nil, err
	}
	var vs []Allocation
	for _, a := range allocs {
		if a.ShipmentID != nil && *a.ShipmentID == sh.ID {
			vs = append(vs, a)
		}
	}
	return &ShipmentView{Shipment: *sh, Milestones: ms, Vehicles: vs}, nil
}
