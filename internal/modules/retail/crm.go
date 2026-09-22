package retail

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/validate"
)

// ---- model ----

type Customer struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	CompanyID   string `gorm:"type:uuid"`
	DisplayName string
	Phone       string
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Customer) TableName() string { return "retail.customers" }

// Lead stages. Forward moves go one step at a time; lost needs a reason; won
// only happens when the vehicle is delivered.
var stages = []string{"new", "contacted", "qualified", "test-drive", "negotiation"}

type Lead struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	CompanyID      string `gorm:"type:uuid"`
	BranchID       string `gorm:"type:uuid"`
	CustomerID     string `gorm:"type:uuid"`
	Source         string
	Stage          string
	AssignedUserID *string `gorm:"type:uuid"`
	LostReason     string
	DealID         *string `gorm:"type:uuid"`
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Lead) TableName() string { return "retail.leads" }

// Open reports whether the lead is still being worked on.
func (l *Lead) Open() bool { return l.Stage != "won" && l.Stage != "lost" }

// Qualified leads can turn into a sale.
func (l *Lead) Qualified() bool {
	return slices.Index(stages, l.Stage) >= slices.Index(stages, "qualified")
}

type Contact struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	LeadID     string `gorm:"type:uuid"`
	Channel    string
	Note       string
	ActorID    string `gorm:"type:uuid"`
	OccurredAt time.Time
}

func (Contact) TableName() string { return "retail.lead_contacts" }

type Task struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	CompanyID   string  `gorm:"type:uuid"`
	CustomerID  string  `gorm:"type:uuid"`
	LeadID      *string `gorm:"type:uuid"`
	DealID      *string `gorm:"type:uuid"`
	OwnerUserID string  `gorm:"type:uuid"`
	DueAt       time.Time
	Title       string
	Status      string
	CompletedAt *time.Time
	CompletedBy *string `gorm:"type:uuid"`
	Version     int64
	CreatedAt   time.Time
}

func (Task) TableName() string { return "retail.tasks" }

// ---- repository ----

type LeadFilter struct {
	CompanyID string
	BranchIDs []string // nil = all branches
	Stage     string
	Limit     int
	Offset    int
}

type CRMRepository interface {
	CreateCustomer(ctx context.Context, c *Customer) error
	Customer(ctx context.Context, companyID, id string) (*Customer, error)
	Customers(ctx context.Context, companyID, query string, limit, offset int) ([]Customer, error)
	UpdateCustomer(ctx context.Context, c *Customer, expected int64) error
	CreateLead(ctx context.Context, l *Lead) error
	Lead(ctx context.Context, companyID, id string) (*Lead, error)
	Leads(ctx context.Context, f LeadFilter) ([]Lead, error)
	UpdateLead(ctx context.Context, l *Lead, expected int64) error
	AddContact(ctx context.Context, c *Contact) error
	Contacts(ctx context.Context, leadID string) ([]Contact, error)
	CreateTask(ctx context.Context, t *Task) error
	Task(ctx context.Context, companyID, id string) (*Task, error)
	Tasks(ctx context.Context, companyID, ownerID, status string, limit, offset int) ([]Task, error)
	UpdateTask(ctx context.Context, t *Task, expected int64) error
}

type crmRepository struct{ db *gorm.DB }

func (r *crmRepository) CreateCustomer(ctx context.Context, c *Customer) error {
	return database.Translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *crmRepository) Customer(ctx context.Context, companyID, id string) (*Customer, error) {
	var c Customer
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&c).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &c, nil
}

func (r *crmRepository) Customers(ctx context.Context, companyID, query string, limit, offset int) ([]Customer, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("display_name, id").Limit(limit).Offset(offset)
	if query != "" {
		q = q.Where("(display_name ILIKE ? OR phone ILIKE ?)", "%"+query+"%", "%"+query+"%")
	}
	cs := []Customer{}
	return cs, database.Translate(q.Find(&cs).Error)
}

func (r *crmRepository) UpdateCustomer(ctx context.Context, c *Customer, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Customer{}, c.ID, expected,
		map[string]any{"display_name": c.DisplayName, "phone": c.Phone, "updated_at": c.UpdatedAt})
	if err == nil {
		c.Version = expected + 1
	}
	return err
}

func (r *crmRepository) CreateLead(ctx context.Context, l *Lead) error {
	return database.Translate(r.db.WithContext(ctx).Create(l).Error)
}

func (r *crmRepository) Lead(ctx context.Context, companyID, id string) (*Lead, error) {
	var l Lead
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&l).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &l, nil
}

func (r *crmRepository) Leads(ctx context.Context, f LeadFilter) ([]Lead, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", f.CompanyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if f.BranchIDs != nil {
		q = q.Where("branch_id IN ?", append(f.BranchIDs, uuid.Nil.String()))
	}
	if f.Stage != "" {
		q = q.Where("stage = ?", f.Stage)
	}
	ls := []Lead{}
	return ls, database.Translate(q.Find(&ls).Error)
}

func (r *crmRepository) UpdateLead(ctx context.Context, l *Lead, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Lead{}, l.ID, expected, map[string]any{
		"stage": l.Stage, "assigned_user_id": l.AssignedUserID, "lost_reason": l.LostReason, "deal_id": l.DealID, "updated_at": l.UpdatedAt})
	if err == nil {
		l.Version = expected + 1
	}
	return err
}

func (r *crmRepository) AddContact(ctx context.Context, c *Contact) error {
	return database.Translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *crmRepository) Contacts(ctx context.Context, leadID string) ([]Contact, error) {
	cs := []Contact{}
	err := r.db.WithContext(ctx).Where("lead_id = ?", leadID).Order("occurred_at, id").Find(&cs).Error
	return cs, database.Translate(err)
}

func (r *crmRepository) CreateTask(ctx context.Context, t *Task) error {
	return database.Translate(r.db.WithContext(ctx).Create(t).Error)
}

func (r *crmRepository) Task(ctx context.Context, companyID, id string) (*Task, error) {
	var t Task
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&t).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &t, nil
}

func (r *crmRepository) Tasks(ctx context.Context, companyID, ownerID, status string, limit, offset int) ([]Task, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("due_at, id").Limit(limit).Offset(offset)
	if ownerID != "" {
		q = q.Where("owner_user_id = ?", ownerID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ts := []Task{}
	return ts, database.Translate(q.Find(&ts).Error)
}

func (r *crmRepository) UpdateTask(ctx context.Context, t *Task, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Task{}, t.ID, expected,
		map[string]any{"status": t.Status, "completed_at": t.CompletedAt, "completed_by": t.CompletedBy})
	if err == nil {
		t.Version = expected + 1
	}
	return err
}

// ---- service ----

type CRMService struct{ deps }

type CustomerInput struct {
	Profile struct {
		DisplayName string `json:"displayName"`
		Phone       string `json:"phone"`
	} `json:"profile"`
}

func (in CustomerInput) apply(v *apperr.Validation, c *Customer) {
	c.DisplayName = validate.Text(v, "profile.displayName", in.Profile.DisplayName, 1, 200)
	c.Phone = validate.Text(v, "profile.phone", in.Profile.Phone, 0, 50)
}

func (s *CRMService) CreateCustomer(ctx context.Context, p *auth.Principal, in CustomerInput) (*Customer, error) {
	var v apperr.Validation
	now := s.clock()
	c := &Customer{ID: uuid.NewString(), CompanyID: p.CompanyID, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(&v, c)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.CRM().CreateCustomer(ctx, c); err != nil {
			return err
		}
		return s.event(ctx, st, p, "customer.created", "customer", c.ID, "", nil)
	})
	return c, err
}

func (s *CRMService) UpdateCustomer(ctx context.Context, p *auth.Principal, id string, expected int64, in CustomerInput) (*Customer, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var c *Customer
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if c, err = st.CRM().Customer(ctx, p.CompanyID, id); err != nil {
			return err
		}
		if c.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		in.apply(&v, c)
		if err := v.Err(); err != nil {
			return err
		}
		c.UpdatedAt = s.clock()
		if err := st.CRM().UpdateCustomer(ctx, c, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "customer.updated", "customer", c.ID, "", nil)
	})
	return c, err
}

func (s *CRMService) Customer(ctx context.Context, p *auth.Principal, id string) (*Customer, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	return s.store.CRM().Customer(ctx, p.CompanyID, id)
}

func (s *CRMService) Customers(ctx context.Context, p *auth.Principal, query string, limit, offset int) ([]Customer, error) {
	return s.store.CRM().Customers(ctx, p.CompanyID, query, limit, offset)
}

type LeadInput struct {
	CustomerID     string  `json:"customerId"`
	BranchID       string  `json:"branchId"`
	Source         string  `json:"source"`
	AssignedUserID *string `json:"assignedUserId"`
}

var sources = []string{"website", "telegram", "phone", "manual"}

// CreateLead opens a lead for an existing customer in a branch. The source
// only records where it came from; no external integration is implied.
func (s *CRMService) CreateLead(ctx context.Context, p *auth.Principal, in LeadInput) (*Lead, error) {
	var v apperr.Validation
	if !slices.Contains(sources, in.Source) {
		v.Add("source", "must be website, telegram, phone or manual")
	}
	if validate.IDs(in.CustomerID) != nil {
		v.Add("customerId", "must be a valid ID")
	}
	if validate.IDs(in.BranchID) != nil {
		v.Add("branchId", "must be a valid ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.branch(ctx, p, "branchId", in.BranchID); err != nil {
		return nil, err
	}
	if in.AssignedUserID != nil {
		if err := s.member(ctx, p, "assignedUserId", *in.AssignedUserID); err != nil {
			return nil, err
		}
	}
	now := s.clock()
	l := &Lead{ID: uuid.NewString(), CompanyID: p.CompanyID, BranchID: in.BranchID, CustomerID: in.CustomerID, Source: in.Source,
		Stage: "new", AssignedUserID: in.AssignedUserID, Version: 1, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.CRM().Customer(ctx, p.CompanyID, in.CustomerID); errors.Is(err, apperr.ErrNotFound) {
			return apperr.FieldError("customerId", "unknown customer")
		} else if err != nil {
			return err
		}
		if err := st.CRM().CreateLead(ctx, l); err != nil {
			return err
		}
		return s.event(ctx, st, p, "lead.created", "lead", l.ID, "", map[string]any{"source": in.Source})
	})
	return l, err
}

func (s *CRMService) lead(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Lead, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	l, err := st.CRM().Lead(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if !inScope(p, l.BranchID) {
		return nil, apperr.ErrNotFound
	}
	if expected >= 0 && l.Version != expected {
		return nil, apperr.ErrStale
	}
	return l, nil
}

// Assign gives an open lead to a company member.
func (s *CRMService) Assign(ctx context.Context, p *auth.Principal, id string, expected int64, userID string) (*Lead, error) {
	if err := s.member(ctx, p, "assignedUserId", userID); err != nil {
		return nil, err
	}
	var l *Lead
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.lead(ctx, st, p, id, expected); err != nil {
			return err
		}
		if !l.Open() {
			return apperr.New(apperr.ErrConflict, "lead_closed", "the lead is closed")
		}
		l.AssignedUserID, l.UpdatedAt = &userID, s.clock()
		if err := st.CRM().UpdateLead(ctx, l, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "lead.assigned", "lead", l.ID, "", map[string]any{"assignedUserId": userID})
	})
	return l, err
}

var channels = []string{"phone", "telegram", "visit", "email", "other"}

// AddContact records a contact with the customer in the lead history.
func (s *CRMService) AddContact(ctx context.Context, p *auth.Principal, id, channel, note string) (*Lead, error) {
	var v apperr.Validation
	if !slices.Contains(channels, channel) {
		v.Add("channel", "must be phone, telegram, visit, email or other")
	}
	note = validate.Text(&v, "note", note, 1, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var l *Lead
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.lead(ctx, st, p, id, -1); err != nil {
			return err
		}
		return st.CRM().AddContact(ctx, &Contact{ID: uuid.NewString(), LeadID: l.ID, Channel: channel, Note: note,
			ActorID: p.UserID, OccurredAt: s.clock()})
	})
	return l, err
}

// SetStage moves a lead one stage forward, or to lost with a reason. Won is
// never set here: it follows delivery of the sold vehicle.
func (s *CRMService) SetStage(ctx context.Context, p *auth.Principal, id string, expected int64, stage, why string) (*Lead, error) {
	var l *Lead
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.lead(ctx, st, p, id, expected); err != nil {
			return err
		}
		if !l.Open() {
			return apperr.New(apperr.ErrConflict, "lead_closed", "the lead is closed")
		}
		switch {
		case stage == "lost":
			var v apperr.Validation
			why = validate.Reason(&v, why)
			if err := v.Err(); err != nil {
				return err
			}
			if l.DealID != nil {
				return apperr.New(apperr.ErrConflict, "lead_in_sale", "cancel the sale first")
			}
			l.LostReason = why
		case stage == "won":
			return apperr.FieldError("stage", "a lead is won when its vehicle is delivered")
		case slices.Index(stages, stage) == slices.Index(stages, l.Stage)+1:
		default:
			return apperr.FieldError("stage", "move one stage forward (next after "+l.Stage+") or to lost")
		}
		l.Stage, l.UpdatedAt = stage, s.clock()
		if err := st.CRM().UpdateLead(ctx, l, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "lead.stage_changed", "lead", l.ID, why, map[string]any{"stage": stage})
	})
	return l, err
}

type LeadView struct {
	Lead     Lead
	Customer Customer
	Contacts []Contact
	History  []Event
}

func (s *CRMService) Lead(ctx context.Context, p *auth.Principal, id string) (*LeadView, error) {
	l, err := s.lead(ctx, s.store, p, id, -1)
	if err != nil {
		return nil, err
	}
	c, err := s.store.CRM().Customer(ctx, p.CompanyID, l.CustomerID)
	if err != nil {
		return nil, err
	}
	contacts, err := s.store.CRM().Contacts(ctx, l.ID)
	if err != nil {
		return nil, err
	}
	history, err := s.store.Events().For(ctx, "lead", l.ID)
	if err != nil {
		return nil, err
	}
	return &LeadView{Lead: *l, Customer: *c, Contacts: contacts, History: history}, nil
}

func (s *CRMService) Leads(ctx context.Context, p *auth.Principal, stage string, limit, offset int) ([]Lead, error) {
	return s.store.CRM().Leads(ctx, LeadFilter{CompanyID: p.CompanyID, BranchIDs: scopeBranches(p), Stage: stage, Limit: limit, Offset: offset})
}

type TaskInput struct {
	CustomerID  string    `json:"customerId"`
	LeadID      *string   `json:"leadId"`
	DealID      *string   `json:"dealId"`
	OwnerUserID string    `json:"ownerUserId"`
	DueAt       time.Time `json:"dueAt"`
	Title       string    `json:"title"`
}

func (s *CRMService) CreateTask(ctx context.Context, p *auth.Principal, in TaskInput) (*Task, error) {
	var v apperr.Validation
	title := validate.Text(&v, "title", in.Title, 1, 300)
	if in.DueAt.IsZero() {
		v.Add("dueAt", "required")
	}
	if validate.IDs(in.CustomerID) != nil {
		v.Add("customerId", "must be a valid ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.member(ctx, p, "ownerUserId", in.OwnerUserID); err != nil {
		return nil, err
	}
	t := &Task{ID: uuid.NewString(), CompanyID: p.CompanyID, CustomerID: in.CustomerID, LeadID: in.LeadID, DealID: in.DealID,
		OwnerUserID: in.OwnerUserID, DueAt: in.DueAt.UTC(), Title: title, Status: "open", Version: 1, CreatedAt: s.clock()}
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.CRM().Customer(ctx, p.CompanyID, in.CustomerID); err != nil {
			return apperr.FieldError("customerId", "unknown customer")
		}
		if in.LeadID != nil {
			l, err := s.lead(ctx, st, p, *in.LeadID, -1)
			if err != nil || l.CustomerID != in.CustomerID {
				return apperr.FieldError("leadId", "not a lead of this customer")
			}
		}
		if in.DealID != nil {
			d, err := st.Deals().Deal(ctx, p.CompanyID, *in.DealID)
			if err != nil || d.CustomerID != in.CustomerID {
				return apperr.FieldError("dealId", "not a sale of this customer")
			}
		}
		if err := st.CRM().CreateTask(ctx, t); err != nil {
			return err
		}
		return s.event(ctx, st, p, "task.created", "task", t.ID, "", nil)
	})
	return t, err
}

func (s *CRMService) CompleteTask(ctx context.Context, p *auth.Principal, id string, expected int64) (*Task, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var t *Task
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if t, err = st.CRM().Task(ctx, p.CompanyID, id); err != nil {
			return err
		}
		if t.Version != expected {
			return apperr.ErrStale
		}
		if t.Status != "open" {
			return apperr.New(apperr.ErrConflict, "task_completed", "the task is already completed")
		}
		now := s.clock()
		t.Status, t.CompletedAt, t.CompletedBy = "completed", &now, &p.UserID
		if err := st.CRM().UpdateTask(ctx, t, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "task.completed", "task", t.ID, "", nil)
	})
	return t, err
}

func (s *CRMService) Tasks(ctx context.Context, p *auth.Principal, owner, status string, limit, offset int) ([]Task, error) {
	if owner == "me" {
		owner = p.UserID
	}
	return s.store.CRM().Tasks(ctx, p.CompanyID, owner, status, limit, offset)
}
