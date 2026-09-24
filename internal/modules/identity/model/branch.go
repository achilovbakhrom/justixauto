package model

import "time"

type Branch struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	CompanyID string `gorm:"type:uuid"`
	Name      string
	Address   string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Branch) TableName() string { return "identity.branches" }
