// Package database opens the shared PostgreSQL connection used by all modules.
package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(url string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true, // "not found" is a normal outcome, not an error
		}),
		TranslateError: true, // unique violations surface as gorm.ErrDuplicatedKey
		NowFunc:        func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	// A span per SQL statement under the request's trace (no-op without OTLP).
	if err := useTracing(db); err != nil {
		return nil, fmt.Errorf("database: tracing: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: pool: %w", err)
	}
	// Every replica holds up to 20 connections: keep replicas × 20 below the
	// server's max_connections (PostgreSQL default 100).
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
