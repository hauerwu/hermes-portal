// Package api: HTTP handlers for the portal's own management API.
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/database"
	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
	"hermesportal/internal/services"
)

// API bundles dependencies for handlers.
type API struct {
	cfg    *config.Config
	db     *gorm.DB
	docker *services.DockerClient
	cache  *services.DashboardSessionCache
	svc    *services.InstanceService
}

// New builds the API.
func New(cfg *config.Config, db *gorm.DB, docker *services.DockerClient,
	cache *services.DashboardSessionCache, svc *services.InstanceService) *API {
	return &API{cfg: cfg, db: db, docker: docker, cache: cache, svc: svc}
}

// ── auth ───────────────────────────────────────────────────────────────

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) Login(c *gin.Context) {
	var body loginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var user models.User
	if err := a.db.Where("username = ?", strings.TrimSpace(body.Username)).First(&user).Error; err != nil ||
		user.PasswordHash == "" || !security.VerifyPassword(body.Password, user.PasswordHash) {
		database.Audit(a.db, nil, nil, "login_failed", body.Username, "", middleware.ClientIP(c))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if !user.Active {
		c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
		return
	}
	database.TouchLastLogin(a.db, user.ID)
	database.Audit(a.db, &user.ID, user.TenantID, "login", user.Username, "", middleware.ClientIP(c))
	a.RespondWithTokens(c, &user)
}

func (a *API) RespondWithTokens(c *gin.Context, user *models.User) {
	access, err := security.MakeAccessToken(a.cfg, user.ID, user.TenantID, user.Role, "access")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	refresh, _ := security.MakeAccessToken(a.cfg, user.ID, user.TenantID, user.Role, "refresh")
	// HttpOnly cookie so the embedded dashboard iframe (same origin) is
	// authenticated without needing an Authorization header.
	middleware.SetPortalSessionCookie(c, access)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "bearer",
		"user":          publicUser(user),
	})
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

func (a *API) Refresh(c *gin.Context) {
	var body refreshBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	claims, err := security.ParseToken(a.cfg, body.RefreshToken)
	if err != nil || claims.Type != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	var user models.User
	if err := a.db.First(&user, "id = ?", claims.Subject).Error; err != nil || !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found or disabled"})
		return
	}
	a.RespondWithTokens(c, &user)
}

func (a *API) Me(c *gin.Context) {
	c.JSON(http.StatusOK, publicUser(middleware.CurrentUser(c)))
}

// ── tenants (super admin) ──────────────────────────────────────────────

type tenantBody struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

func (a *API) ListTenants(c *gin.Context) {
	var tenants []models.Tenant
	a.db.Order("id").Find(&tenants)
	out := make([]gin.H, 0, len(tenants))
	for i := range tenants {
		out = append(out, publicTenant(&tenants[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (a *API) CreateTenant(c *gin.Context) {
	var body tenantBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	slug := services.Slugify(body.Name, body.Slug)
	var count int64
	a.db.Model(&models.Tenant{}).Where("slug = ?", slug).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("tenant slug '%s' exists", slug)})
		return
	}
	t := models.Tenant{Name: body.Name, Slug: slug, Description: body.Description, Settings: "{}"}
	if err := a.db.Create(&t).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user := middleware.CurrentUser(c)
	database.Audit(a.db, &user.ID, &t.ID, "tenant_create", slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusCreated, publicTenant(&t))
}

func (a *API) GetTenant(c *gin.Context) {
	id := mustID(c)
	var t models.Tenant
	if err := a.db.First(&t, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	c.JSON(http.StatusOK, publicTenant(&t))
}

func (a *API) UpdateTenant(c *gin.Context) {
	id := mustID(c)
	var body tenantBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var t models.Tenant
	if err := a.db.First(&t, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	slug := t.Slug
	if body.Slug != "" {
		slug = services.Slugify(body.Name, body.Slug)
	}
	if body.Name != "" {
		t.Name = body.Name
	}
	t.Slug = slug
	t.Description = body.Description
	a.db.Save(&t)
	user := middleware.CurrentUser(c)
	database.Audit(a.db, &user.ID, &t.ID, "tenant_update", slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicTenant(&t))
}

func (a *API) DeleteTenant(c *gin.Context) {
	id := mustID(c)
	user := middleware.CurrentUser(c)
	var t models.Tenant
	if err := a.db.First(&t, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	a.db.Delete(&t)
	database.Audit(a.db, &user.ID, &t.ID, "tenant_delete", t.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── users ──────────────────────────────────────────────────────────────

type userBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	TenantID *uint  `json:"tenant_id"`
	Active   *bool  `json:"active"`
}

func (a *API) ListUsers(c *gin.Context) {
	actor := middleware.CurrentUser(c)
	var users []models.User
	if actor.Role == models.RoleSuperAdmin {
		a.db.Order("id").Find(&users)
	} else {
		a.db.Where("tenant_id = ?", actor.TenantID).Order("id").Find(&users)
	}
	out := make([]gin.H, 0, len(users))
	for i := range users {
		out = append(out, publicUser(&users[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (a *API) CreateUser(c *gin.Context) {
	var body userBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	actor := middleware.CurrentUser(c)
	role := resolveRole(body.Role, actor)
	tenantID, ok := resolveTenant(body.TenantID, actor)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id required for super admin"})
		return
	}
	var count int64
	a.db.Model(&models.User{}).Where("username = ?", body.Username).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}
	hash, _ := security.HashPassword(body.Password)
	active := true
	if body.Active != nil {
		active = *body.Active
	}
	u := models.User{
		TenantID:     &tenantID,
		Username:     body.Username,
		PasswordHash: hash,
		Email:        body.Email,
		Role:         role,
		Active:       active,
	}
	if err := a.db.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, &actor.ID, &tenantID, "user_create", u.Username, "", middleware.ClientIP(c))
	c.JSON(http.StatusCreated, publicUser(&u))
}

func (a *API) UpdateUser(c *gin.Context) {
	id := mustID(c)
	var body userBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	actor := middleware.CurrentUser(c)
	var target models.User
	if err := a.db.First(&target, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && (target.TenantID == nil || *target.TenantID != *actor.TenantID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "user belongs to another tenant"})
		return
	}
	if body.Username != "" {
		target.Username = body.Username
	}
	if body.Email != "" {
		target.Email = body.Email
	}
	if body.Role != "" {
		target.Role = resolveRole(body.Role, actor)
	}
	if body.Active != nil {
		target.Active = *body.Active
	}
	if actor.Role == models.RoleSuperAdmin && body.TenantID != nil {
		target.TenantID = body.TenantID
	}
	if body.Password != "" {
		hash, _ := security.HashPassword(body.Password)
		target.PasswordHash = hash
	}
	a.db.Save(&target)
	database.Audit(a.db, &actor.ID, target.TenantID, "user_update", target.Username, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicUser(&target))
}

func (a *API) DeleteUser(c *gin.Context) {
	id := mustID(c)
	actor := middleware.CurrentUser(c)
	if id == actor.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}
	var target models.User
	if err := a.db.First(&target, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && (target.TenantID == nil || *target.TenantID != *actor.TenantID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "user belongs to another tenant"})
		return
	}
	a.db.Delete(&target)
	database.Audit(a.db, &actor.ID, target.TenantID, "user_delete", target.Username, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func resolveRole(role string, actor *models.User) string {
	switch role {
	case models.RoleMember, models.RoleTenantAdmin:
		if actor.Role == models.RoleSuperAdmin || actor.Role == models.RoleTenantAdmin {
			return role
		}
	case models.RoleSuperAdmin:
		if actor.Role == models.RoleSuperAdmin {
			return role
		}
	}
	return models.RoleMember
}

func resolveTenant(tenantID *uint, actor *models.User) (uint, bool) {
	if actor.Role == models.RoleSuperAdmin {
		if tenantID == nil {
			return 0, false
		}
		return *tenantID, true
	}
	if actor.TenantID == nil {
		return 0, false
	}
	return *actor.TenantID, true
}

// ── instances ──────────────────────────────────────────────────────────

type instanceBody struct {
	Name       string            `json:"name"`
	Mode       string            `json:"mode"`
	Slug       string            `json:"slug"`
	Image      string            `json:"image"`
	RemoteURL  string            `json:"remote_url"`
	OpenAPIURL string            `json:"openapi_url"`
	ExtraEnv   map[string]string `json:"extra_env"`
	MemLimit   string            `json:"mem_limit"`
	TenantID   *uint             `json:"tenant_id"` // super admin only
}

func (a *API) ListInstances(c *gin.Context) {
	actor := middleware.CurrentUser(c)
	var instances []models.Instance
	if actor.Role == models.RoleSuperAdmin {
		a.db.Where("status != ?", models.StatusDestroyed).Order("id").Find(&instances)
	} else if actor.TenantID != nil {
		a.db.Where("tenant_id = ? AND status != ?", *actor.TenantID, models.StatusDestroyed).Order("id").Find(&instances)
	} else {
		instances = []models.Instance{}
	}
	out := make([]gin.H, 0, len(instances))
	for i := range instances {
		out = append(out, publicInstance(&instances[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (a *API) CreateInstance(c *gin.Context) {
	var body instanceBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	actor := middleware.CurrentUser(c)
	tenantID, ok := resolveTenant(body.TenantID, actor)
	if !ok {
		// super admin without explicit tenant → default to first tenant
		var first models.Tenant
		if err := a.db.Order("id").First(&first).Error; err == nil {
			tenantID = first.ID
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no tenant exists"})
			return
		}
	}
	mode := body.Mode
	if mode == "" {
		mode = models.ModeDocker
	}
	inst, err := a.svc.Create(c.Request.Context(), tenantID, body.Name, mode, body.Slug,
		body.Image, body.RemoteURL, body.OpenAPIURL, body.ExtraEnv, &actor.ID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "docker is not reachable") {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, &actor.ID, &tenantID, "instance_create", inst.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusCreated, publicInstance(inst))
}

func (a *API) GetInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	c.JSON(http.StatusOK, publicInstance(instance))
}

type instanceUpdateBody struct {
	Name       *string           `json:"name"`
	Slug       *string           `json:"slug"`
	Image      *string           `json:"image"`
	RemoteURL  *string           `json:"remote_url"`
	OpenAPIURL *string           `json:"openapi_url"`
	ExtraEnv   map[string]string `json:"extra_env"`
	MemLimit   *string           `json:"mem_limit"`
}

func (a *API) UpdateInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	actor := middleware.CurrentUser(c)
	var body instanceUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	changed := false
	if body.Name != nil && *body.Name != "" {
		instance.Name = *body.Name
		changed = true
	}
	if body.Slug != nil && *body.Slug != "" {
		slug := services.Slugify(*body.Slug, *body.Slug)
		var count int64
		a.db.Model(&models.Instance{}).Where("slug = ? AND id != ?", slug, instance.ID).Count(&count)
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("slug '%s' exists", slug)})
			return
		}
		instance.Slug = slug
		changed = true
	}
	if body.Image != nil && *body.Image != "" {
		instance.Image = *body.Image
		changed = true
	}
	if body.RemoteURL != nil {
		instance.RemoteURL = *body.RemoteURL
		changed = true
	}
	if body.OpenAPIURL != nil {
		instance.OpenAPIURL = *body.OpenAPIURL
		changed = true
	}
	if body.ExtraEnv != nil {
		var cfg models.InstanceConfig
		_ = security.UnmarshalJSON(instance.Config, &cfg)
		cfg.ExtraEnv = body.ExtraEnv
		instance.Config = security.MarshalJSON(cfg)
		changed = true
	}
	if body.MemLimit != nil {
		var cfg models.InstanceConfig
		_ = security.UnmarshalJSON(instance.Config, &cfg)
		cfg.MemLimit = *body.MemLimit
		instance.Config = security.MarshalJSON(cfg)
		changed = true
	}
	if changed {
		a.db.Save(instance)
		database.Audit(a.db, &actor.ID, &instance.TenantID, "instance_update", instance.Slug, "", middleware.ClientIP(c))
	}
	// Recreate the container so image/env changes take effect.
	if instance.Mode == models.ModeDocker {
		if err := a.svc.Recreate(c.Request.Context(), instance); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("container recreate failed: %v", err)})
			return
		}
	}
	c.JSON(http.StatusOK, publicInstance(instance))
}

func (a *API) StartInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	if err := a.svc.Start(c.Request.Context(), instance); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, idPtr(middleware.CurrentUser(c).ID), &instance.TenantID, "instance_start", instance.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicInstance(instance))
}

func (a *API) StopInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	if err := a.svc.Stop(c.Request.Context(), instance); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, idPtr(middleware.CurrentUser(c).ID), &instance.TenantID, "instance_stop", instance.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicInstance(instance))
}

func (a *API) RestartInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	if err := a.svc.Restart(c.Request.Context(), instance); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, idPtr(middleware.CurrentUser(c).ID), &instance.TenantID, "instance_restart", instance.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicInstance(instance))
}

func (a *API) DestroyInstance(c *gin.Context) {
	instance := mustInstance(c, a)
	actor := middleware.CurrentUser(c)
	keepVolume := c.Query("keep_volume") == "1"
	a.svc.Destroy(c.Request.Context(), instance, keepVolume)
	database.Audit(a.db, &actor.ID, &instance.TenantID, "instance_destroy", instance.Slug, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) InstanceHealth(c *gin.Context) {
	instance := mustInstance(c, a)
	result := a.svc.Health(c.Request.Context(), instance)
	c.JSON(http.StatusOK, result)
}

func (a *API) InstanceLogs(c *gin.Context) {
	instance := mustInstance(c, a)
	if instance.Mode != models.ModeDocker {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logs available for docker instances only"})
		return
	}
	logs := a.docker.ContainerLogs(c.Request.Context(), instance.ContainerName, 500)
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (a *API) GatewayURLs(c *gin.Context) {
	instance := mustInstance(c, a)
	base := strings.TrimRight(a.cfg.PublicBaseURL, "/")
	channels := map[string]string{}
	for _, ch := range []string{"whatsapp", "webhook", "bluebubbles", "msgraph", "weixin", "qqbot"} {
		channels[ch] = fmt.Sprintf("%s/api/v1/gateway/%s/webhook/%s/", base, instance.Slug, ch)
	}
	c.JSON(http.StatusOK, gin.H{
		"dashboard":    fmt.Sprintf("%s/instances/%d/dashboard/", base, instance.ID),
		"openapi_base": fmt.Sprintf("%s/api/v1/gateway/%s/openapi", base, instance.Slug),
		"openapi_example": gin.H{
			"endpoint": fmt.Sprintf("%s/api/v1/gateway/%s/openapi/v1/chat/completions", base, instance.Slug),
			"headers":  gin.H{"Content-Type": "application/json", "X-API-Key": "<your portal api key>"},
			"body":     gin.H{"model": "hermes", "messages": []gin.H{{"role": "user", "content": "hello"}}},
		},
		"webhook_channels": channels,
	})
}

// ── api keys ───────────────────────────────────────────────────────────

type apiKeyBody struct {
	Name       string     `json:"name"`
	InstanceID *uint      `json:"instance_id"` // nil = tenant-wide
	ExpiresAt  *time.Time `json:"expires_at"`
}

func (a *API) ListAPIKeys(c *gin.Context) {
	actor := middleware.CurrentUser(c)
	var keys []models.ApiKey
	if actor.Role == models.RoleSuperAdmin {
		a.db.Order("id").Find(&keys)
	} else if actor.TenantID != nil {
		a.db.Where("tenant_id = ?", *actor.TenantID).Order("id").Find(&keys)
	} else {
		keys = []models.ApiKey{}
	}
	out := make([]gin.H, 0, len(keys))
	for i := range keys {
		out = append(out, publicAPIKey(&keys[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (a *API) CreateAPIKey(c *gin.Context) {
	var body apiKeyBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	actor := middleware.CurrentUser(c)
	tenantID, ok := resolveTenant(nil, actor)
	if !ok {
		var first models.Tenant
		if err := a.db.Order("id").First(&first).Error; err == nil {
			tenantID = first.ID
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no tenant exists"})
			return
		}
	}
	// Validate instance scope (if given) belongs to the tenant.
	if body.InstanceID != nil {
		var inst models.Instance
		if err := a.db.First(&inst, *body.InstanceID).Error; err != nil || inst.TenantID != tenantID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "instance not found in tenant"})
			return
		}
	}
	plain, err := security.GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	key := models.ApiKey{
		TenantID:   tenantID,
		InstanceID: body.InstanceID,
		Name:       body.Name,
		KeyPrefix:  security.KeyPrefix(plain),
		KeyHash:    security.HashAPIKey(plain),
		Scopes:     `["openapi"]`,
		Active:     true,
		ExpiresAt:  body.ExpiresAt,
		CreatedBy:  &actor.ID,
	}
	if err := a.db.Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	database.Audit(a.db, &actor.ID, &tenantID, "apikey_create", key.Name, "", middleware.ClientIP(c))
	// Return the plaintext once — it is never stored.
	c.JSON(http.StatusCreated, gin.H{
		"key":         plain,
		"key_prefix":  key.KeyPrefix,
		"id":          key.ID,
		"name":        key.Name,
		"instance_id": key.InstanceID,
		"tenant_id":   key.TenantID,
		"expires_at":  key.ExpiresAt,
	})
}

func (a *API) RevokeAPIKey(c *gin.Context) {
	id := mustID(c)
	actor := middleware.CurrentUser(c)
	var key models.ApiKey
	if err := a.db.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && key.TenantID != *actor.TenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "key belongs to another tenant"})
		return
	}
	a.db.Model(&key).Update("active", false)
	database.Audit(a.db, &actor.ID, &key.TenantID, "apikey_revoke", key.Name, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── misc ───────────────────────────────────────────────────────────────

func (a *API) Health(c *gin.Context) {
	dockerOK := a.docker != nil && a.docker.Ping(c.Request.Context()) == nil
	c.JSON(http.StatusOK, gin.H{"ok": true, "docker": dockerOK, "version": "0.1.0"})
}

// ── helpers ────────────────────────────────────────────────────────────

func mustID(c *gin.Context) uint {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id)
}

func idPtr(id uint) *uint { return &id }

// LoadInstance fetches the :id instance, enforces tenant isolation and
// stores it in the gin context for handlers and proxies.
func (a *API) LoadInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := mustID(c)
		var inst models.Instance
		if err := a.db.First(&inst, id).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "instance not found"})
			return
		}
		actor := middleware.CurrentUser(c)
		if actor == nil || (actor.Role != models.RoleSuperAdmin &&
			(actor.TenantID == nil || *actor.TenantID != inst.TenantID)) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "instance belongs to another tenant"})
			return
		}
		c.Set("instance", &inst)
		c.Next()
	}
}

func mustInstance(c *gin.Context, a *API) *models.Instance {
	v, ok := c.Get("instance")
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		c.Abort()
		return nil
	}
	inst, _ := v.(*models.Instance)
	if inst == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		c.Abort()
		return nil
	}
	return inst
}

// publicUser serializes a user for API responses.
func publicUser(u *models.User) gin.H {
	return gin.H{
		"id":         u.ID,
		"tenant_id":  u.TenantID,
		"username":   u.Username,
		"email":      u.Email,
		"role":       u.Role,
		"active":     u.Active,
		"last_login": u.LastLogin,
		"created_at": u.CreatedAt,
	}
}

func publicTenant(t *models.Tenant) gin.H {
	return gin.H{
		"id":          t.ID,
		"name":        t.Name,
		"slug":        t.Slug,
		"description": t.Description,
		"settings":    t.Settings,
		"created_at":  t.CreatedAt,
	}
}

func publicInstance(i *models.Instance) gin.H {
	var cfg models.InstanceConfig
	_ = security.UnmarshalJSON(i.Config, &cfg)
	// Never leak secrets.
	safe := gin.H{"extra_env": cfg.ExtraEnv, "mem_limit": cfg.MemLimit}
	return gin.H{
		"id":             i.ID,
		"tenant_id":      i.TenantID,
		"name":           i.Name,
		"slug":           i.Slug,
		"mode":           i.Mode,
		"image":          i.Image,
		"container_name": i.ContainerName,
		"status":         i.Status,
		"remote_url":     i.RemoteURL,
		"openapi_url":    i.OpenAPIURL,
		"config":         safe,
		"last_heartbeat": i.LastHeartbeat,
		"created_by":     i.CreatedBy,
		"created_at":     i.CreatedAt,
		"updated_at":     i.UpdatedAt,
	}
}

func publicAPIKey(k *models.ApiKey) gin.H {
	return gin.H{
		"id":          k.ID,
		"tenant_id":   k.TenantID,
		"instance_id": k.InstanceID,
		"name":        k.Name,
		"key_prefix":  k.KeyPrefix,
		"scopes":      k.Scopes,
		"active":      k.Active,
		"expires_at":  k.ExpiresAt,
		"last_used":   k.LastUsed,
		"created_by":  k.CreatedBy,
		"created_at":  k.CreatedAt,
	}
}
