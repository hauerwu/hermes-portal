// Package middleware: bearer-token auth and RBAC scoping.
//
// Roles:
//   - super_admin  — no tenant; manages tenants, users and every instance.
//   - tenant_admin — bound to one tenant; manages its users, instances
//     and API keys.
//   - member       — bound to one tenant; read-only, may call the unified
//     gateway for instances of its tenant.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

// ContextUserKey stores the authenticated user in the gin context.
const ContextUserKey = "portal.user"

// ClientIP extracts the client IP honouring X-Forwarded-For.
func ClientIP(c *gin.Context) string {
	if fwd := c.GetHeader("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	return c.ClientIP()
}

// PortalSessionCookie is the HttpOnly cookie the portal sets at login so
// the embedded dashboard iframe (which cannot attach Authorization headers)
// is authenticated on the same origin.
const PortalSessionCookie = "portal_session"

// AuthRequired validates the access token from the Authorization header or
// the portal session cookie, and loads the user.
func AuthRequired(cfg *config.Config, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			if cookie, err := c.Cookie(PortalSessionCookie); err == nil {
				token = cookie
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing bearer token"})
			return
		}
		claims, err := security.ParseToken(cfg, token)
		if err != nil || claims.Type != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		var user models.User
		if err := db.First(&user, "id = ?", claims.Subject).Error; err != nil || !user.Active {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not found or disabled"})
			return
		}
		c.Set(ContextUserKey, &user)
		c.Next()
	}
}

// SetPortalSessionCookie sets the HttpOnly session cookie on a response.
func SetPortalSessionCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     PortalSessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

// ClearPortalSessionCookie removes the session cookie (logout).
func ClearPortalSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     PortalSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// RequireRole restricts access to the given roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil || !allowed[u.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			return
		}
		c.Next()
	}
}

// CurrentUser returns the authenticated user from context.
func CurrentUser(c *gin.Context) *models.User {
	v, ok := c.Get(ContextUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}

func bearerToken(header string) string {
	if h := strings.TrimSpace(header); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
