// Model library (default-model templates) management.
//
// Tenants configure reusable model entries (display name + OpenAI-compatible
// endpoint URL + model name + API key), mark one as the default, and pick one
// when creating instances. The API key never leaves the backend: responses
// return only whether a key is set, and instance creation snapshots the key
// into the instance's own config.
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// ── Model endpoint test ────────────────────────────────────────────────

// TestModel verifies connectivity + credentials against the model's
// OpenAI-compatible endpoint: a minimal 1-token chat completion first,
// falling back to GET /models for endpoints that reject chat requests.
func (a *API) TestModel(c *gin.Context) {
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
	result := testOpenAIEndpoint(m.URL, m.Key, m.Model)
	detail := "ok"
	if !result["ok"].(bool) {
		detail, _ = result["error"].(string)
	}
	database.Audit(a.db, &actor.ID, &m.TenantID, "model_test", m.Name, detail, middleware.ClientIP(c))
	c.JSON(http.StatusOK, result)
}

// testOpenAIEndpoint performs the live probe (no secrets returned).
func testOpenAIEndpoint(baseURL, key, model string) map[string]any {
	start := time.Now()
	elapsed := func() int64 { return time.Since(start).Milliseconds() }
	base := strings.TrimRight(baseURL, "/")

	client := &http.Client{Timeout: 20 * time.Second}

	// 1) Minimal chat completion (max_tokens=1).
	payload, _ := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	})
	req, err := http.NewRequest(http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			return map[string]any{"ok": false, "error": "连接失败: " + doErr.Error(), "elapsed_ms": elapsed()}
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return map[string]any{"ok": true, "method": "chat_completions", "elapsed_ms": elapsed()}
		}
		// Fall through to /models probe for 400/404/405 endpoints.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return map[string]any{
				"ok": false, "status": resp.StatusCode,
				"error": "认证失败（HTTP " + itoa(resp.StatusCode) + "）：检查 API Key", "elapsed_ms": elapsed(),
			}
		}
		_ = body // keep parseable error below
	}

	// 2) Fallback: GET /models.
	req2, err2 := http.NewRequest(http.MethodGet, base+"/models", nil)
	if err2 == nil {
		if key != "" {
			req2.Header.Set("Authorization", "Bearer "+key)
		}
		resp2, doErr := client.Do(req2)
		if doErr != nil {
			return map[string]any{"ok": false, "error": "连接失败: " + doErr.Error(), "elapsed_ms": elapsed()}
		}
		body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 2048))
		resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			return map[string]any{"ok": true, "method": "models", "elapsed_ms": elapsed()}
		}
		if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
			return map[string]any{
				"ok": false, "status": resp2.StatusCode,
				"error": "认证失败（HTTP " + itoa(resp2.StatusCode) + "）：检查 API Key", "elapsed_ms": elapsed(),
			}
		}
		return map[string]any{
			"ok": false, "status": resp2.StatusCode,
			"error":      "端点不可用（HTTP " + itoa(resp2.StatusCode) + "）：" + extractAPIError(body2),
			"elapsed_ms": elapsed(),
		}
	}

	return map[string]any{"ok": false, "error": "无法构造测试请求", "elapsed_ms": elapsed()}
}

func extractAPIError(body []byte) string {
	// OpenAI-style error envelope: {"error": {"message": "..."}} or {"error": "..."}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 {
		var msgObj struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &msgObj) == nil && msgObj.Message != "" {
			return truncateStr(msgObj.Message, 200)
		}
		var msgStr string
		if json.Unmarshal(envelope.Error, &msgStr) == nil && msgStr != "" {
			return truncateStr(msgStr, 200)
		}
	}
	plain := strings.TrimSpace(string(body))
	if plain != "" {
		return truncateStr(plain, 200)
	}
	return "无响应内容"
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
