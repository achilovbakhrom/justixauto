package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/modules/identity/model"
)

type rolePermission struct {
	RoleID     string `gorm:"primaryKey;type:uuid"`
	Permission string `gorm:"primaryKey"`
}

func (rolePermission) TableName() string { return "identity.role_permissions" }

type userRole struct {
	UserID string `gorm:"primaryKey;type:uuid"`
	RoleID string `gorm:"primaryKey;type:uuid"`
}

func (userRole) TableName() string { return "identity.user_roles" }

type RoleRepository struct{ db *gorm.DB }

func (r *RoleRepository) withPermissions(ctx context.Context, roles []model.Role) ([]model.Role, error) {
	if len(roles) == 0 {
		return roles, nil
	}
	ids := make([]string, len(roles))
	for i, role := range roles {
		ids[i] = role.ID
	}
	var rows []rolePermission
	if err := r.db.WithContext(ctx).Where("role_id IN ?", ids).Order("permission").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	byRole := map[string][]string{}
	for _, row := range rows {
		byRole[row.RoleID] = append(byRole[row.RoleID], row.Permission)
	}
	for i := range roles {
		roles[i].Permissions = byRole[roles[i].ID]
		if roles[i].Permissions == nil {
			roles[i].Permissions = []string{}
		}
	}
	return roles, nil
}

func (r *RoleRepository) List(ctx context.Context) ([]model.Role, error) {
	roles := []model.Role{}
	if err := r.db.WithContext(ctx).Order("system_key NULLS LAST, name").Find(&roles).Error; err != nil {
		return nil, translate(err)
	}
	return r.withPermissions(ctx, roles)
}

func (r *RoleRepository) Get(ctx context.Context, id string) (*model.Role, error) {
	roles, err := r.GetMany(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, translate(gorm.ErrRecordNotFound)
	}
	return &roles[0], nil
}

func (r *RoleRepository) GetMany(ctx context.Context, ids []string) ([]model.Role, error) {
	roles := []model.Role{}
	if len(ids) == 0 {
		return roles, nil
	}
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("name").Find(&roles).Error; err != nil {
		return nil, translate(err)
	}
	return r.withPermissions(ctx, roles)
}

func (r *RoleRepository) savePermissions(ctx context.Context, role *model.Role) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("role_id = ?", role.ID).Delete(&rolePermission{}).Error; err != nil {
		return translate(err)
	}
	if len(role.Permissions) == 0 {
		return nil
	}
	rows := make([]rolePermission, len(role.Permissions))
	for i, p := range role.Permissions {
		rows[i] = rolePermission{RoleID: role.ID, Permission: p}
	}
	return translate(db.Create(&rows).Error)
}

func (r *RoleRepository) Create(ctx context.Context, role *model.Role) error {
	if err := r.db.WithContext(ctx).Create(role).Error; err != nil {
		return translate(err)
	}
	return r.savePermissions(ctx, role)
}

func (r *RoleRepository) Update(ctx context.Context, role *model.Role, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &model.Role{}, role.ID, expected, map[string]any{
		"name": role.Name, "scope": role.Scope, "company_kind": role.CompanyKind, "updated_at": role.UpdatedAt,
	})
	if err != nil {
		return err
	}
	role.Version = expected + 1
	return r.savePermissions(ctx, role)
}

func (r *RoleRepository) UserRoles(ctx context.Context, userID string) ([]model.Role, error) {
	roles := []model.Role{}
	err := r.db.WithContext(ctx).
		Joins("JOIN identity.user_roles ur ON ur.role_id = roles.id AND ur.user_id = ?", userID).
		Order("roles.name").Find(&roles).Error
	if err != nil {
		return nil, translate(err)
	}
	return r.withPermissions(ctx, roles)
}

func (r *RoleRepository) SetUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("user_id = ?", userID).Delete(&userRole{}).Error; err != nil {
		return translate(err)
	}
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]userRole, len(roleIDs))
	for i, id := range roleIDs {
		rows[i] = userRole{UserID: userID, RoleID: id}
	}
	return translate(db.Create(&rows).Error)
}

// LockAdminGuard serializes every change that could remove the last
// platform admin (row lock on the platform_admin role, inside a transaction).
func (r *RoleRepository) LockAdminGuard(ctx context.Context) error {
	var role model.Role
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", model.PlatformAdminRoleID).Take(&role).Error
	return translate(err)
}

// CountActivePlatformAdmins counts active users holding platform_admin,
// ignoring excludeUserID.
func (r *RoleRepository) CountActivePlatformAdmins(ctx context.Context, excludeUserID string) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&model.User{}).
		Joins("JOIN identity.user_roles ur ON ur.user_id = users.id AND ur.role_id = ?", model.PlatformAdminRoleID).
		Where("users.status = ?", model.UserActive)
	if excludeUserID != "" {
		q = q.Where("users.id <> ?", excludeUserID)
	}
	if err := translate(q.Count(&n).Error); err != nil {
		return 0, err
	}
	return n, nil
}
