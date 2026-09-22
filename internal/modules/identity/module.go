package identity

import (
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Register wires the module's repository → service → handler chain and mounts
// its routes under /api/v1/identity.
func Register(api *echo.Group, db *gorm.DB) {
	companies := NewCompanyHandler(NewCompanyService(NewCompanyRepository(db), time.Now))
	companies.Routes(api.Group("/identity"))
}
