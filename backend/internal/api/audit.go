// Audit-log querying.
//
// super_admin sees every tenant's log; tenant_admin / member see only
// their own tenant's log. Filters: action (exact), target (LIKE),
// actor (username, LIKE) plus offset/limit pagination.
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hermesportal/internal/middleware"
	"hermesportal/internal/models"
)

// AuditQuery mirrors the query-string filter set of GET /api/audit.
type AuditQuery struct {
	Action string `form:"action"`
	Target string `form:"target"`
	Actor  string `form:"actor"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

// ListAuditLogs returns paged audit entries for the caller's scope.
func (a *API) ListAuditLogs(c *gin.Context) {
	var q AuditQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query"})
		return
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	actor := middleware.CurrentUser(c)
	query := a.db.Model(&models.AuditLog{})
	if actor.Role != models.RoleSuperAdmin {
		if actor.TenantID == nil {
			c.JSON(http.StatusOK, gin.H{"items": []any{}, "total": 0})
			return
		}
		query = query.Where("tenant_id = ?", *actor.TenantID)
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}
	if q.Target != "" {
		query = query.Where("target LIKE ?", "%"+q.Target+"%")
	}
	if q.Actor != "" {
		query = query.Where("actor_id IN (?)",
			a.db.Model(&models.User{}).Select("id").Where("username LIKE ?", "%"+q.Actor+"%"))
	}

	var total int64
	query.Count(&total)

	var rows []models.AuditLog
	if err := query.Order("created_at DESC").Limit(limit).Offset(q.Offset).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Resolve actor usernames in one query. actor_id 0 is the synthetic
	// super-admin API-key actor (middleware.authenticateAPIKey).
	usernames := map[uint]string{0: "apikey"}
	actorIDs := make([]uint, 0, len(rows))
	for i := range rows {
		if rows[i].ActorID != nil {
			actorIDs = append(actorIDs, *rows[i].ActorID)
		}
	}
	if len(actorIDs) > 0 {
		var users []models.User
		a.db.Where("id IN ?", actorIDs).Find(&users)
		for i := range users {
			usernames[users[i].ID] = users[i].Username
		}
	}

	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		actorName := ""
		if r.ActorID != nil {
			actorName = usernames[*r.ActorID] // 0 → "apikey" (synthetic super-admin key actor)
		}
		items = append(items, gin.H{
			"id":         r.ID,
			"tenant_id":  r.TenantID,
			"actor_id":   r.ActorID,
			"actor":      actorName,
			"action":     r.Action,
			"target":     r.Target,
			"detail":     r.Detail,
			"ip":         r.IP,
			"created_at": r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": q.Offset})
}

// AuditActions lists distinct actions for the filter dropdown.
func (a *API) AuditActions(c *gin.Context) {
	actor := middleware.CurrentUser(c)
	query := a.db.Model(&models.AuditLog{}).Distinct("action")
	if actor.Role != models.RoleSuperAdmin && actor.TenantID != nil {
		query = query.Where("tenant_id = ?", *actor.TenantID)
	}
	var actions []string
	query.Order("action").Pluck("action", &actions)
	c.JSON(http.StatusOK, gin.H{"actions": actions})
}

func uintPtrOrZero(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

// NormalizeAuditDetail truncates verbose details for display.
func NormalizeAuditDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 300 {
		return detail[:300] + "…"
	}
	return detail
}
