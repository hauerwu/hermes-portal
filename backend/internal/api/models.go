// Model library (default-model templates) management.
//
// Tenants configure reusable model entries (display name + OpenAI-compatible
// endpoint URL + model name + API key), mark one as the default, and pick one
// when creating instances. The API key never leaves the backend: responses
// return only whether a key is set, and instance creation snapshots the key
// into the instance's own config.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hermesportal/internal/database"
	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
	"hermesportal/internal/services"
)

type modelBody struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Provider  string `json:"provider"`
	URL       string `json:"url"`
	Model     string `json:"model"`
	Key       string `json:"key"`
	IsDefault bool   `json:"is_default"`
	TenantID  *uint  `json:"tenant_id"` // super admin only
}

// ListModels returns the caller-tenant's model library.
func (a *API) ListModels(c *gin.Context) {
	actor := middleware.CurrentUser(c)
	var rows []models.ModelConfig
	if actor.Role == models.RoleSuperAdmin {
		a.db.Order("is_default DESC, id").Find(&rows)
	} else if actor.TenantID != nil {
		a.db.Where("tenant_id = ?", *actor.TenantID).Order("is_default DESC, id").Find(&rows)
	} else {
		rows = []models.ModelConfig{}
	}
	out := make([]gin.H, 0, len(rows))
	for i := range rows {
		out = append(out, publicModelConfig(&rows[i]))
	}
	c.JSON(http.StatusOK, out)
}

// CreateModel adds a model entry to the tenant's library.
func (a *API) CreateModel(c *gin.Context) {
	var body modelBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if body.URL == "" || body.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url and model are required"})
		return
	}
	actor := middleware.CurrentUser(c)
	tenantID, ok := resolveTenant(body.TenantID, actor)
	if !ok {
		var first models.Tenant
		if err := a.db.Order("id").First(&first).Error; err == nil {
			tenantID = first.ID
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no tenant exists"})
			return
		}
	}

	tx := a.db.Begin()
	m := models.ModelConfig{
		TenantID:  tenantID,
		Name:      body.Name,
		Slug:      services.Slugify(body.Slug, body.Name),
		Provider:  body.Provider,
		URL:       body.URL,
		Model:     body.Model,
		Key:       body.Key,
		IsDefault: body.IsDefault,
	}
	if m.IsDefault {
		tx.Model(&models.ModelConfig{}).Where("tenant_id = ?", tenantID).Update("is_default", false)
	}
	if err := tx.Create(&m).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tx.Commit()
	database.Audit(a.db, &actor.ID, &tenantID, "model_create", m.Name, "", middleware.ClientIP(c))
	c.JSON(http.StatusCreated, publicModelConfig(&m))
}

// UpdateModel edits a library entry. Empty key keeps the stored key.
func (a *API) UpdateModel(c *gin.Context) {
	id := mustID(c)
	var body modelBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	actor := middleware.CurrentUser(c)
	var m models.ModelConfig
	if err := a.db.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && m.TenantID != *actor.TenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "model belongs to another tenant"})
		return
	}
	if body.Name != "" {
		m.Name = body.Name
	}
	if body.Slug != "" {
		m.Slug = services.Slugify(body.Slug, body.Slug)
	}
	if body.Provider != "" {
		m.Provider = body.Provider
	}
	if body.URL != "" {
		m.URL = body.URL
	}
	if body.Model != "" {
		m.Model = body.Model
	}
	if body.Key != "" {
		m.Key = body.Key
	}

	tx := a.db.Begin()
	if body.IsDefault && !m.IsDefault {
		tx.Model(&models.ModelConfig{}).Where("tenant_id = ?", m.TenantID).Update("is_default", false)
		m.IsDefault = true
	}
	if err := tx.Save(&m).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tx.Commit()
	database.Audit(a.db, &actor.ID, &m.TenantID, "model_update", m.Name, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicModelConfig(&m))
}

// SetDefaultModel marks one entry as the tenant default (clears the rest).
func (a *API) SetDefaultModel(c *gin.Context) {
	id := mustID(c)
	actor := middleware.CurrentUser(c)
	var m models.ModelConfig
	if err := a.db.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && m.TenantID != *actor.TenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "model belongs to another tenant"})
		return
	}
	tx := a.db.Begin()
	tx.Model(&models.ModelConfig{}).Where("tenant_id = ?", m.TenantID).Update("is_default", false)
	m.IsDefault = true
	tx.Save(&m)
	tx.Commit()
	database.Audit(a.db, &actor.ID, &m.TenantID, "model_default", m.Name, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, publicModelConfig(&m))
}

// DeleteModel removes a library entry.
func (a *API) DeleteModel(c *gin.Context) {
	id := mustID(c)
	actor := middleware.CurrentUser(c)
	var m models.ModelConfig
	if err := a.db.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if actor.Role != models.RoleSuperAdmin && m.TenantID != *actor.TenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "model belongs to another tenant"})
		return
	}
	a.db.Delete(&m)
	database.Audit(a.db, &actor.ID, &m.TenantID, "model_delete", m.Name, "", middleware.ClientIP(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func publicModelConfig(m *models.ModelConfig) gin.H {
	return gin.H{
		"id":         m.ID,
		"tenant_id":  m.TenantID,
		"name":       m.Name,
		"slug":       m.Slug,
		"provider":   m.Provider,
		"url":        m.URL,
		"model":      m.Model,
		"has_key":    m.Key != "",
		"is_default": m.IsDefault,
		"created_at": m.CreatedAt,
	}
}
