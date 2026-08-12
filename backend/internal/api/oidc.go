// OIDC single sign-on (authorization-code flow).
//
// Provider discovery happens at runtime against the issuer's
// .well-known/openid-configuration, so Keycloak, Dex, Okta, Entra ID and
// Auth0 all work without code changes. Users are linked by (issuer, sub);
// when PORTAL_OIDC_AUTO_PROVISION is enabled, unknown SSO users are
// auto-created in the default tenant as members.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"hermesportal/internal/config"
	"hermesportal/internal/database"
	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

type oidcState struct {
	provider *oidc.Provider
	oauth    *oauth2.Config
}

var (
	oidcMu     sync.RWMutex
	oidcClient *oidcState
)

func setOIDCClient(s *oidcState) {
	oidcMu.Lock()
	oidcClient = s
	oidcMu.Unlock()
}

func getOIDCClient() *oidcState {
	oidcMu.RLock()
	defer oidcMu.RUnlock()
	return oidcClient
}

// oidcStateCookie carries the OAuth state between /authorize and /callback so
// the callback can verify it and reject login-CSRF attempts.
const oidcStateCookie = "portal_oidc_state"

func oidcCookieSecure(cfg *config.Config) bool {
	return strings.HasPrefix(strings.ToLower(cfg.PublicBaseURL), "https")
}

// InitOIDC performs provider discovery; safe to call repeatedly (a Settings
// page save re-invokes it to hot-reload the client).
func (a *API) InitOIDC(cfg *config.Config, s *OIDCSettings) error {
	if s == nil {
		s = LoadOIDCSettings(a.db, cfg)
	}
	if !s.Enabled || s.Issuer == "" || s.ClientID == "" {
		setOIDCClient(nil)
		log.Printf("[portal] OIDC disabled")
		return nil
	}
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, s.Issuer)
	if err != nil {
		log.Printf("[portal] OIDC discovery failed: %v", err)
		return err
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if strings.TrimSpace(s.Scopes) != "" {
		scopes = strings.FieldsFunc(s.Scopes, func(r rune) bool { return r == ' ' || r == ',' })
	}
	setOIDCClient(&oidcState{
		provider: provider,
		oauth: &oauth2.Config{
			ClientID:     s.ClientID,
			ClientSecret: s.ClientSecret,
			RedirectURL:  cfg.PublicBaseURL + "/api/auth/oidc/callback",
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
		},
	})
	log.Printf("[portal] OIDC enabled: %s", s.Issuer)
	return nil
}

// isOIDCAdmin reports whether the ID token's admin claim is truthy.
// Truthy values: true, "true", "1", "yes", nonzero numbers, or a list
// containing any truthy element.
func isOIDCAdmin(idToken *oidc.IDToken, claimName string) bool {
	if claimName == "" {
		return false
	}
	var raw map[string]any
	if err := idToken.Claims(&raw); err != nil {
		return false
	}
	v, ok := raw[claimName]
	if !ok {
		return false
	}
	return claimTruthy(v)
}

func claimTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes" || s == "on"
	case float64:
		return t != 0
	case json.Number:
		f, err := t.Float64()
		return err == nil && f != 0
	case []any:
		for _, item := range t {
			if claimTruthy(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (a *API) OIDCAuthorize(c *gin.Context) {
	oc := getOIDCClient()
	if oc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OIDC not configured"})
		return
	}
	state, err := security.RandomSecret(24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "state error"})
		return
	}
	// Remember the state so the callback can verify it (login-CSRF defense).
	// The nonce mirrors the state so the ID token can be checked the same way.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/api/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   oidcCookieSecure(a.cfg),
		MaxAge:   600,
	})
	url := oc.oauth.AuthCodeURL(state, oidc.Nonce(state))
	c.Redirect(http.StatusFound, url)
}

func (a *API) OIDCCallback(c *gin.Context) {
	oc := getOIDCClient()
	if oc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OIDC not configured"})
		return
	}
	code := c.Query("code")
	state := c.Query("state")
	errDesc := c.Query("error_description")
	if c.Query("error") != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errDesc})
		return
	}
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
		return
	}
	// Verify the OAuth state against the value we set at /authorize.
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing state"})
		return
	}
	stateCookie, err := c.Cookie(oidcStateCookie)
	if err != nil || stateCookie != state {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}
	// Consume the state cookie immediately (one-time use).
	http.SetCookie(c.Writer, &http.Cookie{
		Name: oidcStateCookie, Value: "", Path: "/api/auth/oidc",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: oidcCookieSecure(a.cfg), MaxAge: -1,
	})

	ctx := c.Request.Context()
	oauthToken, err := oc.oauth.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token exchange failed"})
		return
	}
	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id_token"})
		return
	}
	sso := LoadOIDCSettings(a.db, a.cfg)
	// Verify against the *effective* client id (page-configurable, persisted in
	// SQLite) — not the stale env var used only as the initial default.
	verifier := oc.provider.Verifier(&oidc.Config{ClientID: sso.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id_token verification failed"})
		return
	}

	var claims struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Nonce             string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "claims parse failed"})
		return
	}
	// The nonce mirrors the OAuth state; reject a token that doesn't echo it.
	if claims.Nonce != "" && claims.Nonce != state {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nonce mismatch"})
		return
	}

	// OIDC admin mapping: when PORTAL_OIDC_ADMIN_CLAIM is configured and its
	// value is truthy for this user, the SSO user gets tenant_admin (existing
	// members are upgraded on login, never downgraded).
	targetRole := models.RoleMember
	if isOIDCAdmin(idToken, sso.AdminClaim) {
		targetRole = models.RoleTenantAdmin
	}
	issuer := sso.Issuer

	var user models.User
	err = a.db.Where("oidc_sub = ? AND oidc_issuer = ?", claims.Sub, issuer).First(&user).Error
	if err != nil {
		// Unknown SSO user: auto-provision or reject.
		if !sso.AutoProvision {
			c.JSON(http.StatusForbidden, gin.H{"error": "SSO user not provisioned in portal"})
			return
		}
		var tenant models.Tenant
		if terr := a.db.Where("slug = ?", "default").First(&tenant).Error; terr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "default tenant missing"})
			return
		}
		username := claims.PreferredUsername
		if username == "" {
			username = claims.Email
		}
		if username == "" {
			username = claims.Sub
		}
		user = models.User{
			TenantID:   &tenant.ID,
			Username:   username,
			Email:      claims.Email,
			Role:       targetRole,
			OIDCSub:    claims.Sub,
			OIDCIssuer: issuer,
			Active:     true,
		}
		if err := a.db.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user provisioning failed"})
			return
		}
	} else if targetRole == models.RoleTenantAdmin && user.Role == models.RoleMember {
		// Existing SSO member now carries the admin claim → upgrade.
		user.Role = models.RoleTenantAdmin
		a.db.Model(&user).Update("role", models.RoleTenantAdmin)
	}
	if !user.Active {
		c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
		return
	}
	database.TouchLastLogin(a.db, user.ID)
	database.Audit(a.db, &user.ID, user.TenantID, "login_oidc", user.Username, "", middleware.ClientIP(c))

	access, _ := security.MakeAccessToken(a.cfg, user.ID, user.TenantID, user.Role, "access")
	refresh, _ := security.MakeAccessToken(a.cfg, user.ID, user.TenantID, user.Role, "refresh")
	// Deliver tokens via the URL fragment so they never land in server logs.
	c.Redirect(http.StatusFound,
		a.cfg.PublicBaseURL+"/#/auth/sso?access_token="+access+"&refresh_token="+refresh)
}

// ssoTokenExchange lets the SPA hand its fragment tokens back to the API
// (which sets the HttpOnly portal cookie for the embedded dashboard).
func (a *API) SSOTokenExchange(c *gin.Context) {
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.AccessToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	claims, err := security.ParseToken(a.cfg, body.AccessToken)
	if err != nil || claims.Type != "access" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	var user models.User
	if err := a.db.First(&user, "id = ?", claims.Subject).Error; err != nil || !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	middleware.SetPortalSessionCookie(c, body.AccessToken)
	c.JSON(http.StatusOK, gin.H{"user": publicUser(&user)})
}

func (a *API) Logout(c *gin.Context) {
	middleware.ClearPortalSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
