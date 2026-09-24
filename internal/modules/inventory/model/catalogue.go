// Package model holds inventory's GORM models, filters and value types.
// It imports only internal/pkg and must never import a sibling package
// (repository, service or handler).
package model

import "time"

// VehicleModel is a make/model/variant; details live in immutable
// specification versions so existing vehicles never change retroactively.
type VehicleModel struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	Make               string
	Model              string
	Variant            string
	CurrentSpecVersion int
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (VehicleModel) TableName() string { return "inventory.vehicle_models" }

type Specification struct {
	ModelID       string `gorm:"primaryKey;type:uuid"`
	SpecVersion   int    `gorm:"primaryKey"`
	Year          int
	BodyType      string
	ExteriorColor string
	InteriorColor string
	Powertrain    string
	Drivetrain    string
	CreatedAt     time.Time
	CreatedBy     string `gorm:"type:uuid"`
}

func (Specification) TableName() string { return "inventory.model_specifications" }

// ModelFilter narrows a vehicle model catalogue listing.
type ModelFilter struct {
	Query         string // matches make, model or variant
	Limit, Offset int
}
