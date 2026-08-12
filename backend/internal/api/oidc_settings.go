// OIDC SSO settings — page-configurable, persisted in SQLite.
//
// Environment variables provide the initial defaults; once saved from the
// Settings page the database copy wins. Saving re-initializes the OIDC
// client in-process (no restart required).
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/database"
	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

const oidcSettingsKey = "oidc"

// OIDCSettings is the persisted SSO configuration.
type OIDCSettings struct {
	Enabled       bool   `json:"enabled"`
	Issuer        string `json:"issuer"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	Scopes        string `json:"scopes"`
	AdminClaim    string `json:"admin_claim"`
	AutoProvision bool   `json:"auto_provision"`
}

// DefaultOIDCSettings derives defaults from environment variables.
func DefaultOIDCSettings(cfg *config.Config) *OIDCSettings {
	return &OIDCSettings{
		Enabled:       cfg.OIDCEnabled,
		Issuer:        cfg.OIDCIssuer,
		ClientID:      cfg.OIDCClientID,
		ClientSecret:  cfg.OIDCClientSecret,
		Scopes:        cfg.OIDCScopes,
		AdminClaim:    cfg.OIDCAdminClaim,
		AutoProvision: cfg.OIDCAutoProvision,
	}
}

// LoadOIDCSettings reads the persisted settings; falls back to env defaults
// when nothing has been saved yet.
func LoadOIDCSettings(db *gorm.DB, cfg *config.Config) *OIDCSettings {
	def := DefaultOIDCSettings(cfg)
	var row models.PortalSetting
	if err := db.First(&row, "key = ?", oidcSettingsKey).Error; err != nil {
		return def
	}
	var saved OIDCSettings
	if err := security.UnmarshalJSON(row.Value, &saved); err != nil {
		return def
	}
	return &saved
}

// SaveOIDCSettings persists the settings row.
func SaveOIDCSettings(db *gorm.DB, s *OIDCSettings) error {
	row := models.PortalSetting{
		Key:   oidcSettingsKey,
		Value: security.MarshalJSON(s),
	}
	// Upsert.
	return db.Where(models.PortalSetting{Key: oidcSettingsKey}).
		Assign(models.PortalSetting{Value: row.Value}).
		FirstOrCreate(&row).Error
}

// OIDCGetSettings returns the current SSO config (super admin edit view).
func (a *API) OIDCGetSettings(c *gin.Context) {
	s := LoadOIDCSettings(a.db, a.cfg)
	redirect := ""
	if s.Issuer != "" {
		redirect = strings.TrimRight(a.cfg.PublicBaseURL, "/") + "/api/auth/oidc/callback"
	}
	// The client secret is a credential — only expose it to the super admin.
	clientSecret := s.ClientSecret
	if middleware.CurrentUser(c).Role != models.RoleSuperAdmin {
		clientSecret = ""
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":        s.Enabled,
		"issuer":         s.Issuer,
		"client_id":      s.ClientID,
		"client_secret":  clientSecret,
		"scopes":         s.Scopes,
		"admin_claim":    s.AdminClaim,
		"auto_provision": s.AutoProvision,
		"redirect_uri":   redirect,
		"editable":       middleware.CurrentUser(c).Role == models.RoleSuperAdmin,
	})
}

// OIDCUpdateSettings saves the SSO config and hot-reloads the client.
func (a *API) OIDCUpdateSettings(c *gin.Context) {
	if middleware.CurrentUser(c).Role != models.RoleSuperAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "super admin required"})
		return
	}
	var body OIDCSettings
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.Issuer = strings.TrimSpace(body.Issuer)
	body.ClientID = strings.TrimSpace(body.ClientID)
	body.ClientSecret = strings.TrimSpace(body.ClientSecret)
	body.Scopes = strings.TrimSpace(body.Scopes)
	body.AdminClaim = strings.TrimSpace(body.AdminClaim)
	if body.Issuer == "" || body.ClientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "issuer and client_id are required"})
		return
	}
	if err := SaveOIDCSettings(a.db, &body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Hot-reload the OIDC client (or disable it) without restarting.
	if err := a.InitOIDC(a.cfg, &body); err != nil {
		// Discovery failed — persist the config anyway so the operator can
		// fix it from the page, but tell them.
		database.Audit(a.db, &middleware.CurrentUser(c).ID, nil, "oidc_update_error",
			body.Issuer, err.Error(), middleware.ClientIP(c))
		c.JSON(http.StatusOK, gin.H{
			"ok":           true,
			"discovery_ok": false,
			"error":        err.Error(),
		})
		return
	}
	database.Audit(a.db, &middleware.CurrentUser(c).ID, nil, "oidc_update", body.Issuer,
		"", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true, "discovery_ok": true})
}

// OIDCStatus is the lightweight public status used by the login page.
func (a *API) OIDCStatus(c *gin.Context) {
	s := LoadOIDCSettings(a.db, a.cfg)
	c.JSON(http.StatusOK, gin.H{
		"enabled":        getOIDCClient() != nil,
		"issuer":         s.Issuer,
		"admin_claim":    s.AdminClaim,
		"auto_provision": s.AutoProvision,
	})
}
