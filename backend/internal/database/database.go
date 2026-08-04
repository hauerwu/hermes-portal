// Package database wires GORM + SQLite and seeds the bootstrap data.
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
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
		&models.PortalSetting{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// Manual migrations AutoMigrate cannot express on SQLite.
	if err := migrateAPIKeyTenantNullable(db); err != nil {
		return nil, fmt.Errorf("migrate api_keys: %w", err)
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

// migrateAPIKeyTenantNullable upgrades legacy api_keys tables whose
// tenant_id column is NOT NULL (pre-two-tier schema) to nullable — the
// global super-admin key tier stores tenant_id = NULL. SQLite cannot
// alter a column's nullability, so the table is rebuilt preserving rows
// and indexes. No-op when the column is already nullable or absent.
func migrateAPIKeyTenantNullable(db *gorm.DB) error {
	cols, err := tableColumns(db, "api_keys")
	if err != nil {
		return err
	}
	hasTenant, tenantNotNull := false, false
	for _, c := range cols {
		if c.name == "tenant_id" {
			hasTenant, tenantNotNull = true, c.notNull
		}
	}
	if !hasTenant || !tenantNotNull {
		return nil // already migrated (or table not created yet)
	}
	// Rebuild with the model's full column set (tenant_id nullable,
	// including updated_at which older schemas lack). Existing columns
	// are copied verbatim; new ones default to NULL.
	const create = `CREATE TABLE "api_keys_mig" (
  "id" integer PRIMARY KEY AUTOINCREMENT,
  "tenant_id" integer,
  "instance_id" integer,
  "name" text NOT NULL,
  "key_prefix" text NOT NULL,
  "key_hash" text NOT NULL,
  "scopes" text NOT NULL DEFAULT '["openapi"]',
  "active" numeric DEFAULT true,
  "expires_at" datetime,
  "last_used" datetime,
  "created_by" integer,
  "created_at" datetime,
  "updated_at" datetime
)`
	// Copy only columns that already exist in the legacy table.
	existing := make([]string, 0, len(cols))
	for _, c := range cols {
		existing = append(existing, `"`+c.name+`"`)
	}
	colList := strings.Join(existing, ",")
	log.Printf("[portal] migrating api_keys: tenant_id NOT NULL → nullable (rebuild table)")
	stmts := []string{
		create,
		`INSERT INTO "api_keys_mig" (` + colList + `) SELECT ` + colList + ` FROM "api_keys";`,
		`DROP TABLE "api_keys";`,
		`ALTER TABLE "api_keys_mig" RENAME TO "api_keys";`,
		`CREATE INDEX "idx_api_keys_tenant_id" ON "api_keys"("tenant_id");`,
		`CREATE INDEX "idx_api_keys_instance_id" ON "api_keys"("instance_id");`,
		`CREATE UNIQUE INDEX "idx_api_keys_key_hash" ON "api_keys"("key_hash");`,
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, s := range stmts {
			if err := tx.Exec(s).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// tableColumns returns the columns of a table via PRAGMA table_info.
type tableColumn struct {
	name    string
	notNull bool
}

func tableColumns(db *gorm.DB, table string) ([]tableColumn, error) {
	var raw []struct {
		Name    string `gorm:"column:name"`
		NotNull int    `gorm:"column:notnull"`
	}
	if err := db.Raw("PRAGMA table_info(" + table + ")").Scan(&raw).Error; err != nil {
		return nil, err
	}
	out := make([]tableColumn, 0, len(raw))
	for _, r := range raw {
		out = append(out, tableColumn{name: r.Name, notNull: r.NotNull == 1})
	}
	return out, nil
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
