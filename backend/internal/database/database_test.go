package database

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestMigrateAPIKeyTenantNullable simulates a legacy api_keys table
// (tenant_id NOT NULL, no updated_at) and asserts the rebuild keeps rows,
// drops the NOT NULL constraint and recreates indexes.
func TestMigrateAPIKeyTenantNullable(t *testing.T) {
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "m.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	const legacy = `CREATE TABLE "api_keys" (
  "id" integer PRIMARY KEY AUTOINCREMENT,
  "tenant_id" integer NOT NULL,
  "instance_id" integer,
  "name" text NOT NULL,
  "key_prefix" text NOT NULL,
  "key_hash" text NOT NULL,
  "scopes" text NOT NULL DEFAULT '["openapi"]',
  "active" numeric DEFAULT true,
  "created_at" datetime
);
CREATE INDEX "idx_api_keys_tenant_id" ON "api_keys"("tenant_id");
CREATE UNIQUE INDEX "idx_api_keys_key_hash" ON "api_keys"("key_hash");
INSERT INTO "api_keys" ("tenant_id","name","key_prefix","key_hash") VALUES (1,'legacy','hp_ab','deadbeef');`
	for _, s := range []string{legacy} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := migrateAPIKeyTenantNullable(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Row preserved.
	var n int64
	db.Table("api_keys").Count(&n)
	if n != 1 {
		t.Fatalf("rows after migrate = %d, want 1", n)
	}

	// tenant_id now nullable.
	cols, err := tableColumns(db, "api_keys")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cols {
		if c.name == "tenant_id" {
			found = true
			if c.notNull {
				t.Fatal("tenant_id still NOT NULL after migrate")
			}
		}
	}
	if !found {
		t.Fatal("tenant_id column missing")
	}

	// updated_at column added (part of the model).
	hasUpdated := false
	for _, c := range cols {
		if c.name == "updated_at" {
			hasUpdated = true
		}
	}
	if !hasUpdated {
		t.Fatal("updated_at column missing after migrate")
	}

	// A global (NULL tenant) key can now be inserted.
	if err := db.Exec(`INSERT INTO "api_keys" ("tenant_id","name","key_prefix","key_hash") VALUES (NULL,'global','hp_xy','cafe01')`).Error; err != nil {
		t.Fatalf("insert global key: %v", err)
	}

	// Idempotent: second run is a no-op.
	if err := migrateAPIKeyTenantNullable(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
