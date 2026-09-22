package identity

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	Get(ctx context.Context, id string) (*User, error)
	FindByLogin(ctx context.Context, login string) (*User, error)
	// EmailOrLoginTaken reports whether another user already uses the email or login.
	EmailOrLoginTaken(ctx context.Context, email string, login *string) (bool, error)
	// LoginTaken reports whether another user already signs in with login.
	LoginTaken(ctx context.Context, login, exceptUserID string) (bool, error)
	List(ctx context.Context, limit, offset int) ([]User, error)
	Update(ctx context.Context, u *User, expected int64) error
	// SetLoginState records failed attempts and lockouts without bumping the
	// version, so sign-in attempts never make an admin's edit stale.
	SetLoginState(ctx context.Context, id string, failed int, lockedUntil *time.Time) error
	// RecordFailure counts one failed attempt atomically (safe across
	// replicas); at the threshold it locks the account and resets the count.
	RecordFailure(ctx context.Context, id string, threshold int, lockUntil time.Time) error
}

type userRepository struct{ db *gorm.DB }

func (r *userRepository) Create(ctx context.Context, u *User) error {
	return translate(r.db.WithContext(ctx).Create(u).Error)
}

func (r *userRepository) Get(ctx context.Context, id string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

func (r *userRepository) FindByLogin(ctx context.Context, login string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).Where("lower(login) = lower(?)", login).Take(&u).Error; err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

func (r *userRepository) EmailOrLoginTaken(ctx context.Context, email string, login *string) (bool, error) {
	q := r.db.WithContext(ctx).Model(&User{}).Where("lower(email) = lower(?)", email)
	if login != nil {
		q = q.Or("lower(login) = lower(?)", *login)
	}
	var n int64
	return n > 0, translate(q.Count(&n).Error)
}

func (r *userRepository) LoginTaken(ctx context.Context, login, exceptUserID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("lower(login) = lower(?) AND id <> ?", login, exceptUserID).Count(&n).Error
	return n > 0, translate(err)
}

func (r *userRepository) List(ctx context.Context, limit, offset int) ([]User, error) {
	limit, offset = pageDefaults(limit, offset)
	users := []User{}
	err := r.db.WithContext(ctx).Order("display_name, id").Limit(limit).Offset(offset).Find(&users).Error
	return users, translate(err)
}

func (r *userRepository) Update(ctx context.Context, u *User, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &User{}, u.ID, expected, map[string]any{
		"display_name": u.DisplayName, "email": u.Email, "login": u.Login, "password_hash": u.PasswordHash, "password_change_required": u.PasswordChangeRequired,
		"status": u.Status, "status_reason": u.StatusReason, "updated_at": u.UpdatedAt,
	})
	if err == nil {
		u.Version = expected + 1
	}
	return err
}

func (r *userRepository) RecordFailure(ctx context.Context, id string, threshold int, lockUntil time.Time) error {
	err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(map[string]any{
		"failed_logins": gorm.Expr("CASE WHEN failed_logins + 1 >= ? THEN 0 ELSE failed_logins + 1 END", threshold),
		"locked_until":  gorm.Expr("CASE WHEN failed_logins + 1 >= ? THEN ?::timestamptz ELSE locked_until END", threshold, lockUntil),
	}).Error
	return translate(err)
}

func (r *userRepository) SetLoginState(ctx context.Context, id string, failed int, lockedUntil *time.Time) error {
	err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).
		Updates(map[string]any{"failed_logins": failed, "locked_until": lockedUntil}).Error
	return translate(err)
}
