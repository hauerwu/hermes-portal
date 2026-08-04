package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"hermesportal/internal/config"
	"hermesportal/internal/database"
	"hermesportal/internal/models"
	"hermesportal/internal/router"
	"hermesportal/internal/security"
	"hermesportal/internal/services"
)

// newTestServer boots an in-memory portal (SQLite in a temp dir, no docker).
func newTestServer(t *testing.T) (*gin.Engine, *config.Config, *models.User, *models.User) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.DBPath = filepath.Join(dir, "test.db")
	cfg.AdminUsername = "root"
	cfg.AdminPassword = "root-pass"
	cfg.JWTSecret = "integration-test-secret"
	cfg.PublicBaseURL = "http://portal.test"

	os.Setenv("PORTAL_STATIC_DIR", "/nonexistent")
	t.Cleanup(func() { os.Unsetenv("PORTAL_STATIC_DIR") })

	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("db: %v", err)
	}

	// Second tenant + tenant admin + member.
	tenant2 := models.Tenant{Name: "Tenant Two", Slug: "tenant-two", Settings: "{}"}
	if err := db.Create(&tenant2).Error; err != nil {
		t.Fatal(err)
	}
	h, _ := security.HashPassword("admin-pass")
	admin2 := models.User{TenantID: &tenant2.ID, Username: "admin2", PasswordHash: h, Role: models.RoleTenantAdmin, Active: true}
	member := models.User{TenantID: &tenant2.ID, Username: "member1", PasswordHash: h, Role: models.RoleMember, Active: true}
	if err := db.Create(&admin2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}

	// A remote instance for tenant2.
	cfg2, _ := services.GenerateInstanceConfig("inst2")
	inst := models.Instance{
		TenantID: tenant2.ID, Name: "Inst Two", Slug: "inst-two",
		Mode: models.ModeRemote, Status: models.StatusRunning,
		Config: security.MarshalJSON(cfg2), RemoteURL: "http://remote.example:9119",
		OpenAPIURL: "http://remote.example:8642/v1",
	}
	if err := db.Create(&inst).Error; err != nil {
		t.Fatal(err)
	}

	// A remote instance in the default tenant (tenant 1).
	var t1 models.Tenant
	if err := db.Where("slug = ?", "default").First(&t1).Error; err != nil {
		t.Fatal(err)
	}
	instOne := models.Instance{
		TenantID: t1.ID, Name: "Inst One", Slug: "inst-one",
		Mode: models.ModeRemote, Status: models.StatusRunning,
		Config: "{}", RemoteURL: "http://one.example:9119",
		OpenAPIURL: "http://one.example:8642/v1",
	}
	if err := db.Create(&instOne).Error; err != nil {
		t.Fatal(err)
	}

	engine := router.New(cfg, db)

	var rootUser, tenantUser models.User
	db.Where("username = ?", "root").First(&rootUser)
	db.Where("username = ?", "admin2").First(&tenantUser)
	return engine, cfg, &rootUser, &tenantUser
}

func doReq(t *testing.T, engine *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf.Write(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func tokenFor(t *testing.T, cfg *config.Config, u *models.User) string {
	t.Helper()
	tok, err := security.MakeAccessToken(cfg, u.ID, u.TenantID, u.Role, "access")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestLoginAndTenantIsolation(t *testing.T) {
	engine, cfg, root, admin2 := newTestServer(t)

	// Login with wrong password → 401.
	w := doReq(t, engine, "POST", "/api/auth/login", "", map[string]string{"username": "admin2", "password": "nope"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("login wrong pw: %d", w.Code)
	}

	rootTok := tokenFor(t, cfg, root)
	adminTok := tokenFor(t, cfg, admin2)

	// Super admin sees both tenants.
	w = doReq(t, engine, "GET", "/api/tenants", rootTok, nil)
	if w.Code != 200 {
		t.Fatalf("list tenants: %d %s", w.Code, w.Body.String())
	}
	var tenants []map[string]any
	json.Unmarshal(w.Body.Bytes(), &tenants)
	if len(tenants) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(tenants))
	}

	// Tenant admin cannot list tenants (403).
	w = doReq(t, engine, "GET", "/api/tenants", adminTok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant admin list tenants: %d", w.Code)
	}

	// Tenant admin sees only its own instances.
	w = doReq(t, engine, "GET", "/api/instances", adminTok, nil)
	var instances []map[string]any
	json.Unmarshal(w.Body.Bytes(), &instances)
	if len(instances) != 1 || instances[0]["slug"] != "inst-two" {
		t.Fatalf("tenant admin instances: %v", instances)
	}

	// Tenant admin cannot create a tenant-scoped key for tenant 1 (tenant_id forced).
	w = doReq(t, engine, "POST", "/api/apikeys", adminTok, map[string]any{"name": "k", "tenant_id": 1})
	var keyResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &keyResp)
	if keyResp["tenant_id"] != float64(instances[0]["tenant_id"].(float64)) {
		t.Fatalf("apikey tenant mismatch: %v", keyResp)
	}
}

func TestGatewayAPIKeyAuth(t *testing.T) {
	engine, cfg, root, admin2 := newTestServer(t)
	rootTok := tokenFor(t, cfg, root)

	// Create an API key for tenant2's instance via the tenant admin.
	w := doReq(t, engine, "POST", "/api/apikeys", tokenFor(t, cfg, admin2),
		map[string]any{"name": "gw-key", "instance_id": 1})
	if w.Code != 200 && w.Code != 201 {
		t.Fatalf("create key: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Key string `json:"key"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)

	// Wrong key → 401.
	w = doReq(t, engine, "GET", "/api/v1/gateway/inst-two/openapi/v1/models", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", w.Code)
	}

	// Valid key → the upstream is unreachable in tests → expect 502 (auth passed).
	w = doReq(t, engine, "GET", "/api/v1/gateway/inst-two/openapi/v1/models", "", nil)
	_ = w
	req := httptest.NewRequest("GET", "/api/v1/gateway/inst-two/openapi/v1/models", nil)
	req.Header.Set("X-API-Key", created.Key)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("valid key rejected: %d", rec.Code)
	}

	// Instance-scoped key must NOT access another tenant's instance (inst-one
	// belongs to tenant 1, the key is scoped to tenant 2's inst-two).
	instOneReq := httptest.NewRequest("GET", "/api/v1/gateway/inst-one/openapi/v1/models", nil)
	instOneReq.Header.Set("X-API-Key", created.Key)
	recOne := httptest.NewRecorder()
	engine.ServeHTTP(recOne, instOneReq)
	if recOne.Code != http.StatusUnauthorized && recOne.Code != http.StatusForbidden {
		t.Fatalf("instance-scoped key must be rejected on another tenant's instance: %d", recOne.Code)
	}

	// Super admin creates a global key (tenant_id = null) that may access
	// any tenant's instance.
	w = doReq(t, engine, "POST", "/api/apikeys", rootTok, map[string]string{"name": "root-key"})
	if w.Code != 200 && w.Code != 201 {
		t.Fatalf("root key: %d", w.Code)
	}
	var rootKey struct {
		Key      string `json:"key"`
		TenantID *uint  `json:"tenant_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &rootKey)
	if rootKey.TenantID != nil {
		t.Fatalf("global key must have tenant_id = null, got %v", *rootKey.TenantID)
	}
	req2 := httptest.NewRequest("GET", "/api/v1/gateway/inst-two/openapi/v1/models", nil)
	req2.Header.Set("X-API-Key", rootKey.Key)
	rec2 := httptest.NewRecorder()
	engine.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusUnauthorized || rec2.Code == http.StatusForbidden {
		t.Fatalf("global key rejected on other tenant's instance: %d", rec2.Code)
	}

	// ── Global super-admin key also authenticates the management API ──
	// /api/tenants is super-admin-only and requires portal auth.
	me := httptest.NewRequest("GET", "/api/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+rootKey.Key)
	meRec := httptest.NewRecorder()
	engine.ServeHTTP(meRec, me)
	if meRec.Code != http.StatusOK {
		t.Fatalf("global key /api/auth/me: %d", meRec.Code)
	}
	var meBody struct {
		Role string `json:"role"`
	}
	json.Unmarshal(meRec.Body.Bytes(), &meBody)
	if meBody.Role != "super_admin" {
		t.Fatalf("global key actor role: %q", meBody.Role)
	}
	reqTenants := httptest.NewRequest("GET", "/api/tenants", nil)
	reqTenants.Header.Set("X-API-Key", rootKey.Key)
	recTenants := httptest.NewRecorder()
	engine.ServeHTTP(recTenants, reqTenants)
	if recTenants.Code != http.StatusOK {
		t.Fatalf("global key management API /api/tenants: %d %s", recTenants.Code, recTenants.Body.String())
	}

	// Instance-scoped key must NOT access the management API.
	keyMgmt := httptest.NewRequest("GET", "/api/tenants", nil)
	keyMgmt.Header.Set("Authorization", "Bearer "+created.Key)
	keyMgmtRec := httptest.NewRecorder()
	engine.ServeHTTP(keyMgmtRec, keyMgmt)
	if keyMgmtRec.Code != http.StatusForbidden && keyMgmtRec.Code != http.StatusUnauthorized {
		t.Fatalf("instance-scoped key must be rejected on management API: %d", keyMgmtRec.Code)
	}
}

func TestDashboardProxyAuth(t *testing.T) {
	engine, cfg, _, admin2 := newTestServer(t)
	adminTok := tokenFor(t, cfg, admin2)

	// Without portal auth → 401.
	w := doReq(t, engine, "GET", "/instances/1/dashboard/", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard unauthenticated: %d", w.Code)
	}

	// With auth → upstream unreachable in tests → 502 (proxy engaged).
	w = doReq(t, engine, "GET", "/instances/1/dashboard/", adminTok, nil)
	if w.Code == http.StatusUnauthorized {
		t.Fatalf("dashboard auth failed with token")
	}
}

func TestAuditLogScoping(t *testing.T) {
	engine, cfg, root, admin2 := newTestServer(t)
	rootTok := tokenFor(t, cfg, root)
	adminTok := tokenFor(t, cfg, admin2)

	// tenant-2 rows: admin2 + member1 logins
	doReq(t, engine, "POST", "/api/auth/login", "",
		map[string]string{"username": "admin2", "password": "admin-pass"})
	doReq(t, engine, "POST", "/api/auth/login", "",
		map[string]string{"username": "member1", "password": "admin-pass"})
	// tenant-1 row: super admin creates a tenant
	doReq(t, engine, "POST", "/api/tenants", rootTok, map[string]string{"name": "Tenant Three"})

	w := doReq(t, engine, "GET", "/api/audit", rootTok, nil)
	if w.Code != 200 {
		t.Fatalf("audit root: %d", w.Code)
	}
	var res struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Total < 3 {
		t.Fatalf("super admin should see all rows, got %d", res.Total)
	}

	// Tenant admin sees only tenant-2 rows (3, never tenant-1/3).
	w = doReq(t, engine, "GET", "/api/audit", adminTok, nil)
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Total != 2 {
		t.Fatalf("tenant admin should see 2 tenant-2 rows, got %d", res.Total)
	}
	for _, item := range res.Items {
		if item["tenant_id"] != float64(2) {
			t.Fatalf("tenant leak: %v", item["tenant_id"])
		}
	}

	// Action filter works.
	w = doReq(t, engine, "GET", "/api/audit?action=login", rootTok, nil)
	json.Unmarshal(w.Body.Bytes(), &res)
	for _, item := range res.Items {
		if item["action"] != "login" {
			t.Fatalf("filter mismatch: %v", item["action"])
		}
	}

	// Actor filter works (member1).
	w = doReq(t, engine, "GET", "/api/audit?actor=member1", rootTok, nil)
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Total != 1 {
		t.Fatalf("actor filter: got %d rows", res.Total)
	}

	// /api/audit/actions returns the distinct set.
	w = doReq(t, engine, "GET", "/api/audit/actions", rootTok, nil)
	if w.Code != 200 {
		t.Fatalf("audit actions: %d", w.Code)
	}
}
