// Package database wires GORM + SQLite and seeds the bootstrap data.
package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"hermesportal/internal/config"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

// Open opens (and migrates) the SQLite database, then seeds defaults.
func Open(cfg *config.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	gormLogLevel := logger.Warn
	if cfg.Debug {
		gormLogLevel = logger.Info
	}
	db, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1) // SQLite: one writer at a time; WAL for readers

	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.Instance{},
		&models.ApiKey{},
		&models.AuditLog{},
		&models.ModelConfig{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := Seed(db, cfg); err != nil {
		return nil, err
	}
	// Release slugs of destroyed instances destroyed before this fix, so
	// their names can be reused for new instances.
	db.Model(&models.Instance{}).
		Where("status = ? AND slug NOT LIKE ?", models.StatusDestroyed, "%-del-%").
		Update("slug", gorm.Expr("slug || '-del-' || id"))
	return db, nil
}

// Seed creates the default tenant and the bootstrap super admin.
func Seed(db *gorm.DB, cfg *config.Config) error {
	var tenant models.Tenant
	if err := db.Where("slug = ?", "default").First(&tenant).Error; err != nil {
		tenant = models.Tenant{
			Name:        "Default Tenant",
			Slug:        "default",
			Description: "Bootstrap tenant",
			Settings:    "{}",
		}
		if err := db.Create(&tenant).Error; err != nil {
			return fmt.Errorf("seed tenant: %w", err)
		}
	}

	var count int64
	db.Model(&models.User{}).Where("username = ?", cfg.AdminUsername).Count(&count)
	if count == 0 {
		if cfg.AdminPassword == "" {
			return fmt.Errorf("PORTAL_ADMIN_PASSWORD must be set on first boot to create the super admin")
		}
		hash, err := security.HashPassword(cfg.AdminPassword)
		if err != nil {
			return fmt.Errorf("hash admin password: %w", err)
		}
		admin := models.User{
			TenantID:     &tenant.ID,
			Username:     cfg.AdminUsername,
			PasswordHash: hash,
			Email:        "admin@local",
			Role:         models.RoleSuperAdmin,
			Active:       true,
		}
		if err := db.Create(&admin).Error; err != nil {
			return fmt.Errorf("seed admin: %w", err)
		}
		log.Printf("[portal] seeded super admin %q (tenant=%s)", cfg.AdminUsername, tenant.Slug)
	}
	return nil
}

// TouchLastLogin updates a user's last-login timestamp.
func TouchLastLogin(db *gorm.DB, userID uint) {
	now := time.Now()
	db.Model(&models.User{}).Where("id = ?", userID).Update("last_login", &now)
}

// Audit writes an audit log row (best effort).
func Audit(db *gorm.DB, actorID, tenantID *uint, action, target, detail, ip string) {
	row := models.AuditLog{
		ActorID:   actorID,
		TenantID:  tenantID,
		Action:    action,
		Target:    target,
		Detail:    detail,
		IP:        ip,
		CreatedAt: time.Now(),
	}
	db.Create(&row)
}
