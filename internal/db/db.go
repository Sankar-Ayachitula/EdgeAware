// Package db opens the GORM connection and applies the schema.
package db

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"edgeaware/internal/models"
)

// Open connects to Postgres through GORM (pgx driver underneath).
func Open(dsn string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return gdb, nil
}

// Migrate creates or updates all tables. GORM's AutoMigrate only adds tables, columns
// and indexes; it never drops anything.
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(
		&models.Role{},
		&models.User{},
		&models.UserHierarchy{},
		&models.Task{},
		&models.SyncFeed{},
		&models.Device{},
		&models.ProcessedOp{},
	); err != nil {
		return err
	}
	// One global, ever-increasing counter for the sync feed.
	return gdb.Exec(`CREATE SEQUENCE IF NOT EXISTS sync_seq`).Error
}
