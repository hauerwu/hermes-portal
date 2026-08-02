// Package router wires the HTTP surface: management API, OIDC SSO, the
// unified gateway and the embedded-dashboard reverse proxy.
package router

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hermesportal/internal/api"
	"hermesportal/internal/config"
	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
	"hermesportal/internal/proxy"
	"hermesportal/internal/services"
)

// StaticDir is where the built React SPA lives (overridable at runtime).
var StaticDir = os.Getenv("PORTAL_STATIC_DIR")

// New builds the gin engine with every route registered.
func New(cfg *config.Config, db *gorm.DB) *gin.Engine {
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(cfg))

	docker, err := services.NewDockerClient(cfg.DockerHost)
	if err != nil {
		log.Printf("[portal] docker client init: %v (local instances unavailable)", err)
		docker = nil
	}
	cache := services.NewDashboardSessionCache(cfg, db)
	svc := services.NewInstanceService(cfg, db, docker, cache)
	apiHandler := api.New(cfg, db, docker, cache, svc)
	apiHandler.InitOIDC(cfg)

	dashboardProxy := proxy.NewDashboardProxy(cfg, db, cache)
	gatewayProxy := proxy.NewGatewayProxy(cfg, db)

	auth := middleware.AuthRequired(cfg, db)

	// ── Public routes ──────────────────────────────────────────────────
	r.GET("/api/health", apiHandler.Health)
	r.POST("/api/auth/login", apiHandler.Login)
	r.POST("/api/auth/refresh", apiHandler.Refresh)
	r.GET("/api/auth/oidc/status", apiHandler.OIDCStatus)
	r.GET("/api/auth/oidc/authorize", apiHandler.OIDCAuthorize)
	r.GET("/api/auth/oidc/callback", apiHandler.OIDCCallback)
	r.POST("/api/auth/sso/exchange", apiHandler.SSOTokenExchange)
	r.POST("/api/auth/logout", apiHandler.Logout)
	r.GET("/api/auth/me", auth, apiHandler.Me)

	// ── Unified gateway (API-key authenticated at the handler) ─────────
	// Resolve the instance by slug; tenant/instance scope is enforced by
	// the caller's portal API key itself, so no portal login is required.
	gatewayResolver := func(c *gin.Context) {
		slug := c.Param("slug")
		var inst models.Instance
		if err := db.Where("slug = ?", slug).First(&inst).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
			c.Abort()
			return
		}
		c.Set("instance", &inst)
		c.Next()
	}
	gateway := r.Group("/api/v1/gateway", gatewayResolver)
	{
		gateway.GET("/:slug/openapi/v1/*path", gatewayProxy.OpenAPIHandler())
		gateway.POST("/:slug/openapi/v1/*path", gatewayProxy.OpenAPIHandler())
		gateway.DELETE("/:slug/openapi/v1/*path", gatewayProxy.OpenAPIHandler())
		gateway.PUT("/:slug/openapi/v1/*path", gatewayProxy.OpenAPIHandler())
		gateway.GET("/:slug/webhook/:channel/*path", gatewayProxy.WebhookHandler())
		gateway.POST("/:slug/webhook/:channel/*path", gatewayProxy.WebhookHandler())
		gateway.PUT("/:slug/webhook/:channel/*path", gatewayProxy.WebhookHandler())
		gateway.DELETE("/:slug/webhook/:channel/*path", gatewayProxy.WebhookHandler())
	}
	// ── Management API (portal login required) ─────────────────────────
	mgmt := r.Group("/api", auth)
	{
		// tenants — super admin
		sa := mgmt.Group("/tenants", middleware.RequireRole(models.RoleSuperAdmin))
		sa.GET("", apiHandler.ListTenants)
		sa.POST("", apiHandler.CreateTenant)
		sa.GET("/:id", apiHandler.GetTenant)
		sa.PUT("/:id", apiHandler.UpdateTenant)
		sa.DELETE("/:id", apiHandler.DeleteTenant)

		// users — tenant admin (or super admin)
		users := mgmt.Group("/users", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin))
		users.GET("", apiHandler.ListUsers)
		users.POST("", apiHandler.CreateUser)
		users.PUT("/:id", apiHandler.UpdateUser)
		users.DELETE("/:id", apiHandler.DeleteUser)

		// instances — tenant admin manages; member reads
		instances := mgmt.Group("/instances")
		instances.GET("", apiHandler.ListInstances)
		instances.POST("", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.CreateInstance)
		inst := instances.Group("/:id", apiHandler.LoadInstance())
		inst.GET("", apiHandler.GetInstance)
		inst.GET("/health", apiHandler.InstanceHealth)
		inst.GET("/logs", apiHandler.InstanceLogs)
		inst.GET("/gateway-urls", apiHandler.GatewayURLs)
		inst.POST("/start", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.StartInstance)
		inst.POST("/stop", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.StopInstance)
		inst.POST("/restart", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.RestartInstance)
		inst.DELETE("", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.DestroyInstance)
		inst.PUT("", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin), apiHandler.UpdateInstance)

		// api keys — tenant admin (or super admin)
		keys := mgmt.Group("/apikeys", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTenantAdmin))
		keys.GET("", apiHandler.ListAPIKeys)
		keys.POST("", apiHandler.CreateAPIKey)
		keys.DELETE("/:id", apiHandler.RevokeAPIKey)

		// audit log — any logged-in user (scoped to own tenant)
		audit := mgmt.Group("/audit")
		audit.GET("", apiHandler.ListAuditLogs)
		audit.GET("/actions", apiHandler.AuditActions)
	}

	// ── Embedded dashboard proxy (portal login via cookie/header) ──────
	dash := r.Group("/instances/:id/dashboard", auth, apiHandler.LoadInstance())
	dash.Any("/*path", dashboardProxy.Handler())

	// ── SPA (history fallback) ─────────────────────────────────────────
	mountSPA(r, StaticDir)
	return r
}

// mountSPA serves the built React app and falls back to index.html for
// client-side routes (except /api/*).
func mountSPA(r *gin.Engine, staticDir string) {
	if staticDir == "" {
		staticDir = "./static"
	}
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
		log.Printf("[portal] no SPA build at %s — API-only mode", staticDir)
		return
	}
	r.Static("/assets", filepath.Join(staticDir, "assets"))
	r.GET("/", spaHandler(staticDir))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
			strings.HasPrefix(c.Request.URL.Path, "/instances/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		spaHandler(staticDir)(c)
	})
}

func spaHandler(staticDir string) gin.HandlerFunc {
	index := filepath.Join(staticDir, "index.html")
	return func(c *gin.Context) {
		c.File(index)
	}
}

func requestLogger(cfg *config.Config) gin.HandlerFunc {
	if cfg.Debug {
		return gin.Logger()
	}
	return func(c *gin.Context) {
		c.Next()
	}
}
