package model

import "time"

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
