package security

import (
	"strings"
	"testing"

	"hermesportal/internal/config"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "scrypt$") {
		t.Fatalf("hash envelope mismatch: %s", hash)
	}
	if !VerifyPassword("s3cret-password", hash) {
		t.Fatal("VerifyPassword should accept the correct password")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("VerifyPassword must reject a wrong password")
	}
	// Hermes-compatible envelope format check (scrypt$n$r$p$salt$dk).
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected 6 parts, got %d", len(parts))
	}
}

func uintPtr(v uint) *uint { return &v }

func testConfig() *config.Config {
	cfg := config.Load()
	cfg.JWTSecret = "unit-test-secret-unit-test-secret"
	return cfg
}

func TestTokens(t *testing.T) {
	cfg := testConfig()
	access, err := MakeAccessToken(cfg, 7, uintPtr(3), "tenant_admin", "access")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(cfg, access)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != "7" || claims.Role != "tenant_admin" || claims.TenantID == nil || *claims.TenantID != 3 {
		t.Fatalf("claims mismatch: %+v", claims)
	}
	if claims.Type != "access" {
		t.Fatalf("type mismatch: %s", claims.Type)
	}
	// Tampered token must fail.
	tampered := access[:len(access)-2] + "xx"
	if _, err := ParseToken(cfg, tampered); err == nil {
		t.Fatal("tampered token must be rejected")
	}
}

func TestAPIKeys(t *testing.T) {
	plain, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, APIKeyPrefix) {
		t.Fatalf("prefix mismatch: %s", plain)
	}
	if HashAPIKey(plain) == HashAPIKey(plain+"x") {
		t.Fatal("hash collision")
	}
	if len(KeyPrefix(plain)) != 8 {
		t.Fatalf("prefix len: %d", len(KeyPrefix(plain)))
	}
}
