package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// CRM manages customers, leads and follow-up tasks.
type CRM struct{ Deps }

type CustomerInput struct {
	Profile struct {
		DisplayName string `json:"displayName"`
		Phone       string `json:"phone"`
	} `json:"profile"`
}

func (in CustomerInput) apply(v *apperr.Validation, c *model.Customer) {
	c.DisplayName = validate.Text(v, "profile.displayName", in.Profile.DisplayName, 1, 200)
	c.Phone = validate.Text(v, "profile.phone", in.Profile.Phone, 0, 50)
}

func (s *CRM) CreateCustomer(ctx context.Context, p *auth.Principal, in CustomerInput) (*model.Customer, error) {
	var v apperr.Validation
	now := s.clock()
	c := &model.Customer{ID: uuid.NewString(), CompanyID: p.CompanyID, Version: 1, CreatedAt: now, UpdatedAt: now}
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

func (s *CRM) UpdateCustomer(ctx context.Context, p *auth.Principal, id string, expected int64, in CustomerInput) (*model.Customer, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var c *model.Customer
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

func (s *CRM) Customer(ctx context.Context, p *auth.Principal, id string) (*model.Customer, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	return s.store.CRM().Customer(ctx, p.CompanyID, id)
}

func (s *CRM) Customers(ctx context.Context, p *auth.Principal, query string, limit, offset int) ([]model.Customer, error) {
	return s.store.CRM().Customers(ctx, p.CompanyID, query, limit, offset)
}

type LeadInput struct {
	CustomerID     string  `json:"customerId"`
	BranchID       string  `json:"branchId"`
	Source         string  `json:"source"`
	AssignedUserID *string `json:"assignedUserId"`
}

// CreateLead opens a lead for an existing customer in a branch. The source
// only records where it came from; no external integration is implied.
func (s *CRM) CreateLead(ctx context.Context, p *auth.Principal, in LeadInput) (*model.Lead, error) {
	var v apperr.Validation
	if !slices.Contains(model.Sources, in.Source) {
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
	l := &model.Lead{
		ID: uuid.NewString(), CompanyID: p.CompanyID, BranchID: in.BranchID, CustomerID: in.CustomerID, Source: in.Source,
		Stage: "new", AssignedUserID: in.AssignedUserID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
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

func (s *CRM) lead(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*model.Lead, error) {
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
func (s *CRM) Assign(ctx context.Context, p *auth.Principal, id string, expected int64, userID string) (*model.Lead, error) {
	if err := s.member(ctx, p, "assignedUserId", userID); err != nil {
		return nil, err
	}
	var l *model.Lead
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

// AddContact records a contact with the customer in the lead history.
func (s *CRM) AddContact(ctx context.Context, p *auth.Principal, id, channel, note string) (*model.Lead, error) {
	var v apperr.Validation
	if !slices.Contains(model.Channels, channel) {
		v.Add("channel", "must be phone, telegram, visit, email or other")
	}
	note = validate.Text(&v, "note", note, 1, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var l *model.Lead
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.lead(ctx, st, p, id, -1); err != nil {
			return err
		}
		return st.CRM().AddContact(ctx, &model.Contact{
			ID: uuid.NewString(), LeadID: l.ID, Channel: channel, Note: note,
			ActorID: p.UserID, OccurredAt: s.clock(),
		})
	})
	return l, err
}

// SetStage moves a lead one stage forward, or to lost with a reason. Won is
// never set here: it follows delivery of the sold vehicle.
func (s *CRM) SetStage(ctx context.Context, p *auth.Principal, id string, expected int64, stage, why string) (*model.Lead, error) {
	var l *model.Lead
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
		case slices.Index(model.Stages, stage) == slices.Index(model.Stages, l.Stage)+1:
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
	Lead     model.Lead
	Customer model.Customer
	Contacts []model.Contact
	History  []model.Event
}

func (s *CRM) Lead(ctx context.Context, p *auth.Principal, id string) (*LeadView, error) {
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

func (s *CRM) Leads(ctx context.Context, p *auth.Principal, stage string, limit, offset int) ([]model.Lead, error) {
	return s.store.CRM().Leads(ctx, model.LeadFilter{CompanyID: p.CompanyID, BranchIDs: scopeBranches(p), Stage: stage, Limit: limit, Offset: offset})
}

type TaskInput struct {
	CustomerID  string    `json:"customerId"`
	LeadID      *string   `json:"leadId"`
	DealID      *string   `json:"dealId"`
	OwnerUserID string    `json:"ownerUserId"`
	DueAt       time.Time `json:"dueAt"`
	Title       string    `json:"title"`
}

func (s *CRM) CreateTask(ctx context.Context, p *auth.Principal, in TaskInput) (*model.Task, error) {
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
	t := &model.Task{
		ID: uuid.NewString(), CompanyID: p.CompanyID, CustomerID: in.CustomerID, LeadID: in.LeadID, DealID: in.DealID,
		OwnerUserID: in.OwnerUserID, DueAt: in.DueAt.UTC(), Title: title, Status: "open", Version: 1, CreatedAt: s.clock(),
	}
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

func (s *CRM) CompleteTask(ctx context.Context, p *auth.Principal, id string, expected int64) (*model.Task, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var t *model.Task
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

func (s *CRM) Tasks(ctx context.Context, p *auth.Principal, owner, status string, limit, offset int) ([]model.Task, error) {
	if owner == "me" {
		owner = p.UserID
	}
	return s.store.CRM().Tasks(ctx, p.CompanyID, owner, status, limit, offset)
}
