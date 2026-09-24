package model

import (
	"time"

	"justixauto/internal/pkg/money"
)

type Listing struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	CompanyID        string `gorm:"type:uuid"`
	VehicleID        string `gorm:"type:uuid"`
	Text             string
	AskingPriceMinor string
	Currency         string
	Status           string // draft | published | withdrawn
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Listing) TableName() string { return "retail.listings" }

func (l *Listing) Price() money.Money {
	return money.Money{AmountMinor: l.AskingPriceMinor, Currency: l.Currency}
}
