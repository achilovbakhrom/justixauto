package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/identity/model"
)

type UserRepository struct{ db *gorm.DB }

func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	return translate(r.db.WithContext(ctx).Create(u).Error)
}

func (r *UserRepository) Get(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

func (r *UserRepository) FindByLogin(ctx context.Context, login string) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).Where("lower(login) = lower(?)", login).Take(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// EmailOrLoginTaken reports whether another user already uses the email or
// login. An empty email never counts as taken: it is optional and many users
// may have none (user decision 2026-09-26).
func (r *UserRepository) EmailOrLoginTaken(ctx context.Context, email string, login *string) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.User{})
	switch {
	case email != "" && login != nil:
		q = q.Where("lower(email) = lower(?) OR lower(login) = lower(?)", email, *login)
	case email != "":
		q = q.Where("lower(email) = lower(?)", email)
	case login != nil:
		q = q.Where("lower(login) = lower(?)", *login)
	default:
		return false, nil
	}
	var n int64
	return n > 0, translate(q.Count(&n).Error)
}

// LoginTaken reports whether another user already signs in with login.
func (r *UserRepository) LoginTaken(ctx context.Context, login, exceptUserID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("lower(login) = lower(?) AND id <> ?", login, exceptUserID).Count(&n).Error
	return n > 0, translate(err)
}

func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	limit, offset = pageDefaults(limit, offset)
	users := []model.User{}
	err := r.db.WithContext(ctx).Order("display_name, id").Limit(limit).Offset(offset).Find(&users).Error
	return users, translate(err)
}

func (r *UserRepository) Update(ctx context.Context, u *model.User, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &model.User{}, u.ID, expected, map[string]any{
		"display_name": u.DisplayName, "email": u.Email, "login": u.Login, "password_hash": u.PasswordHash, "password_change_required": u.PasswordChangeRequired,
		"status": u.Status, "status_reason": u.StatusReason, "updated_at": u.UpdatedAt,
	})
	if err == nil {
		u.Version = expected + 1
	}
	return err
}

// RecordFailure counts one failed attempt atomically (safe across
// replicas); at the threshold it locks the account and resets the count.
func (r *UserRepository) RecordFailure(ctx context.Context, id string, threshold int, lockUntil time.Time) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"failed_logins": gorm.Expr("CASE WHEN failed_logins + 1 >= ? THEN 0 ELSE failed_logins + 1 END", threshold),
		"locked_until":  gorm.Expr("CASE WHEN failed_logins + 1 >= ? THEN ?::timestamptz ELSE locked_until END", threshold, lockUntil),
	}).Error
	return translate(err)
}

// SetLoginState records failed attempts and lockouts without bumping the
// version, so sign-in attempts never make an admin's edit stale.
func (r *UserRepository) SetLoginState(ctx context.Context, id string, failed int, lockedUntil *time.Time) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"failed_logins": failed, "locked_until": lockedUntil}).Error
	return translate(err)
}
