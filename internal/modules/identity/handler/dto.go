package handler

import (
	"encoding/json"
	"strconv"
	"time"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/modules/identity/service"
)

// revision formats an optimistic-locking version for an ETag/If-Match value.
func revision(v int64) string { return strconv.FormatInt(v, 10) }

// Response DTOs. Models never go to JSON directly, so internal fields such as
// password hashes or login counters cannot leak.

type companyDTO struct {
	ID           string              `json:"id"`
	Kind         model.CompanyKind   `json:"kind"`
	Name         string              `json:"name"`
	LegalName    string              `json:"legalName"`
	Country      service.Label       `json:"country"`
	Region       *service.Label      `json:"region"`
	Registration string              `json:"registration"`
	Email        string              `json:"email"`
	Address      string              `json:"address"`
	Phone        string              `json:"phone"`
	Access       model.CompanyAccess `json:"access"`
	AccessReason string              `json:"accessReason"`
	Revision     string              `json:"revision"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
}

func toCompany(c *model.Company) companyDTO {
	d := companyDTO{
		ID: c.ID, Kind: c.Kind, Name: c.Name, LegalName: c.LegalName,
		Country: service.Label{Key: c.CountryKey, Label: c.Country}, Registration: c.RegistrationNumber,
		Email: c.Email, Address: c.Address, Phone: c.Phone, Access: c.Status, AccessReason: c.StatusReason,
		Revision: revision(c.Version), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.Region != "" {
		d.Region = &service.Label{Key: c.RegionKey, Label: c.Region}
	}
	return d
}

type userDTO struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"displayName"`
	Email        string            `json:"email"`
	Login        *string           `json:"login"`
	Status       model.UserStatus  `json:"status"`
	StatusReason string            `json:"statusReason"`
	Roles        []service.RoleRef `json:"roles"`
	Revision     string            `json:"revision"`
	CreatedAt    time.Time         `json:"createdAt"`
}

func toUser(d *service.UserDetail) userDTO {
	u := d.User
	out := userDTO{
		ID: u.ID, DisplayName: u.DisplayName, Email: u.Email, Login: u.Login, Status: u.Status,
		StatusReason: u.StatusReason, Roles: make([]service.RoleRef, len(d.Roles)), Revision: revision(u.Version), CreatedAt: u.CreatedAt,
	}
	for i, r := range d.Roles {
		out.Roles[i] = service.RoleRef{ID: r.ID, Name: r.Name}
	}
	return out
}

type roleDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	System         bool     `json:"system"`
	PermissionKeys []string `json:"permissionKeys"`
	Revision       string   `json:"revision"`
}

func toRole(r *model.Role) roleDTO {
	perms := model.EffectivePermissions(*r)
	if perms == nil {
		perms = []string{}
	}
	return roleDTO{ID: r.ID, Name: r.Name, System: r.System(), PermissionKeys: perms, Revision: revision(r.Version)}
}

type membershipDTO struct {
	ID           string                    `json:"id"`
	UserID       string                    `json:"userId"`
	CompanyID    string                    `json:"companyId"`
	Status       model.MembershipStatus    `json:"status"`
	BranchAccess service.BranchAccessInput `json:"branchAccess"`
	StatusReason string                    `json:"statusReason"`
	Revision     string                    `json:"revision"`
}

func toMembership(m *model.Membership) membershipDTO {
	ids := m.BranchIDs
	if ids == nil {
		ids = []string{}
	}
	return membershipDTO{
		ID: m.ID, UserID: m.UserID, CompanyID: m.CompanyID, Status: m.Status,
		BranchAccess: service.BranchAccessInput{Mode: m.BranchAccess, BranchIDs: ids}, StatusReason: m.StatusReason,
		Revision: revision(m.Version),
	}
}

type branchDTO struct {
	ID        string `json:"id"`
	CompanyID string `json:"companyId"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Revision  string `json:"revision"`
}

func toBranch(b *model.Branch) branchDTO {
	return branchDTO{ID: b.ID, CompanyID: b.CompanyID, Name: b.Name, Address: b.Address, Revision: revision(b.Version)}
}

type auditDTO struct {
	ID           string          `json:"id"`
	OccurredAt   time.Time       `json:"occurredAt"`
	ActorID      *string         `json:"actorId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   string          `json:"resourceId"`
	CompanyID    *string         `json:"companyId"`
	Reason       string          `json:"reason"`
	Details      json.RawMessage `json:"details"`
}

func toAudit(e *model.AuditEvent) auditDTO {
	return auditDTO{
		ID: e.ID, OccurredAt: e.OccurredAt, ActorID: e.ActorUserID, Action: e.Action,
		ResourceType: e.ResourceType, ResourceID: e.ResourceID, CompanyID: e.CompanyID, Reason: e.Reason,
		Details: json.RawMessage(e.Details),
	}
}

func mapSlice[T, D any](items []T, f func(*T) D) []D {
	out := make([]D, len(items))
	for i := range items {
		out[i] = f(&items[i])
	}
	return out
}
