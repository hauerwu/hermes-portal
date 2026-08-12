// Package proxy: reverse proxies that embed hermes dashboards and expose
// the unified gateway (OpenAI API + channel webhooks).
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
	"hermesportal/internal/services"
)

// bodyBufferLimit — request bodies up to this size are buffered so a
// failed (401) proxied request can be retried with a fresh session.
const bodyBufferLimit = 8 << 20

// DashboardProxy embeds a hermes-agent dashboard under
// /instances/{id}/dashboard. It injects the portal-held session cookie,
// captures Set-Cookie (session refresh), strips it from the browser and
// re-authenticates transparently on 401. WebSockets pass through.
type DashboardProxy struct {
	cfg   *config.Config
	db    *gorm.DB
	cache *services.DashboardSessionCache
}

// NewDashboardProxy builds the dashboard reverse proxy.
func NewDashboardProxy(cfg *config.Config, db *gorm.DB, cache *services.DashboardSessionCache) *DashboardProxy {
	return &DashboardProxy{cfg: cfg, db: db, cache: cache}
}

type retryKey struct{}

// Handler returns a gin handler that serves one instance's dashboard.
func (p *DashboardProxy) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.MustGet("instance").(*models.Instance)
		if instance.Status == models.StatusDestroyed {
			c.JSON(http.StatusNotFound, gin.H{"error": "instance destroyed"})
			return
		}
		prefix := services.Prefix(instance.ID)
		target := p.targetFor(instance)
		if target == nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "instance has no dashboard URL"})
			return
		}

		rp := httputil.NewSingleHostReverseProxy(target)
		rp.FlushInterval = -1 // flush immediately (SSE / streaming)
		rp.ErrorLog = log.New(io.Discard, "", 0)

		rt := &retryRoundTripper{
			base:       http.DefaultTransport,
			proxy:      p,
			instanceID: instance.ID,
		}
		rp.Transport = rt

		rp.Director = func(req *http.Request) {
			// Rewrite the target (scheme/host) — NewSingleHostReverseProxy's
			// default Director does this; our custom one must too.
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Strip the portal prefix; keep the rest of the path.
			rawPath := req.URL.Path
			if strings.HasPrefix(rawPath, prefix) {
				rawPath = strings.TrimPrefix(rawPath, prefix)
			}
			if rawPath == "" {
				rawPath = "/"
			}
			req.URL.Path = rawPath
			req.URL.RawPath = ""

			// The dashboard SPA reads X-Forwarded-Prefix to build its base path.
			req.Header.Set("X-Forwarded-Prefix", prefix)
			scheme := "http"
			if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
				scheme = "https"
			}
			req.Header.Set("X-Forwarded-Proto", scheme)

			// Portal-held session cookie. The instance must never see the
			// portal's own Authorization header (it would be treated as a
			// dashboard bearer token and rejected) — cookie is the auth.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			if cookie := p.cache.CookieHeader(req.Context(), instance); cookie != "" {
				req.Header.Set("Cookie", cookie)
			}

			// Buffer small bodies so 401-retries can replay them.
			if req.Body != nil && req.GetBody == nil && req.ContentLength <= bodyBufferLimit {
				body, err := io.ReadAll(req.Body)
				if err == nil {
					req.Body.Close()
					req.Body = io.NopCloser(bytes.NewReader(body))
					req.GetBody = func() (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(body)), nil
					}
					req.ContentLength = int64(len(body))
				}
			}
		}

		rp.ModifyResponse = func(resp *http.Response) error {
			// Consume Set-Cookie from the instance (session refresh),
			// never forward it to the browser.
			if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
				p.cache.Capture(instance.ID, cookies)
				resp.Header.Del("Set-Cookie")
			}
			// Keep instance redirects inside the portal prefix.
			if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, prefix) {
				resp.Header.Set("Location", prefix+loc)
			}
			return nil
		}

		rp.ServeHTTP(c.Writer, c.Request)
	}
}

func (p *DashboardProxy) targetFor(instance *models.Instance) *url.URL {
	base := services.InstanceDashboardURL(p.cache, instance)
	u, err := url.Parse(base)
	if err != nil {
		return nil
	}
	return u
}

// retryRoundTripper transparently re-authenticates on a 401 (dashboard
// session expiry) by re-logging-in and replaying the request once.
type retryRoundTripper struct {
	base       http.RoundTripper
	proxy      *DashboardProxy
	instanceID uint
}

func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusUnauthorized || isUpgrade(req) {
		return resp, nil
	}
	if req.Context().Value(retryKey{}) != nil {
		return resp, nil // already retried
	}
	if req.GetBody == nil && req.Body != nil {
		return resp, nil // cannot replay the body
	}

	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	instance := rt.loadInstance()
	rt.proxy.cache.Invalidate(rt.instanceID)
	if instance == nil {
		return resp, nil
	}

	r2 := req.Clone(req.Context())
	r2.Header.Del("Authorization")
	r2.Header.Del("Cookie")
	if cookie := rt.proxy.cache.CookieHeader(req.Context(), instance); cookie != "" {
		r2.Header.Set("Cookie", cookie)
	}
	r2 = r2.WithContext(context.WithValue(r2.Context(), retryKey{}, true))
	if r2.Body != nil && r2.GetBody != nil {
		body, _ := r2.GetBody()
		r2.Body = body
	}
	return rt.base.RoundTrip(r2)
}

func (rt *retryRoundTripper) loadInstance() *models.Instance {
	var instance models.Instance
	if err := rt.proxy.db.First(&instance, rt.instanceID).Error; err != nil {
		return nil
	}
	return &instance
}

func isUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Connection"), "upgrade")
}

func now() time.Time {
	return time.Now()
}

// ── Unified gateway ────────────────────────────────────────────────────

// GatewayProxy exposes stable public URLs for instance APIs:
//
//	/api/v1/gateway/{slug}/openapi/v1/...  → instance api_server (:8642)
//	/api/v1/gateway/{slug}/webhook/{ch}/... → instance webhook server
type GatewayProxy struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewGatewayProxy builds the unified gateway.
func NewGatewayProxy(cfg *config.Config, db *gorm.DB) *GatewayProxy {
	return &GatewayProxy{cfg: cfg, db: db}
}

// OpenAPIHandler proxies OpenAI-format requests. The caller must present a
// portal API key (X-API-Key or Authorization: Bearer); the portal swaps it
// for the instance's private API_SERVER_KEY.
func (g *GatewayProxy) OpenAPIHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.MustGet("instance").(*models.Instance)
		key := apiKeyFromRequest(c)
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing API key (X-API-Key header)"})
			return
		}
		if err := g.authorizeKey(c, key, instance); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		var cfg models.InstanceConfig
		_ = security.UnmarshalJSON(instance.Config, &cfg)
		base := g.openAPIBase(instance)
		target, err := url.Parse(base)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "bad openapi target"})
			return
		}

		const marker = "/openapi"
		rp := g.baseProxy(target, func(req *http.Request) {
			req.URL.Path = stripGatewayPrefix(req.URL.Path, instance.Slug, marker)
			req.URL.RawPath = ""
			req.Header.Set("Authorization", "Bearer "+cfg.APIServerKey)
			req.Header.Del("X-API-Key")
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			}
		})
		rp.ServeHTTP(c.Writer, c.Request)
	}
}

// WebhookHandler relays channel callbacks to the instance's webhook server.
// External providers authenticate themselves via their own HMAC / verify
// tokens, which the instance validates.
func (g *GatewayProxy) WebhookHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.MustGet("instance").(*models.Instance)
		channel := c.Param("channel")
		port, basePath := WebhookTarget(channel)
		target, err := url.Parse(webhookBaseURL(instance, port))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "bad target"})
			return
		}

		marker := "/webhook/" + channel
		rp := g.baseProxy(target, func(req *http.Request) {
			req.URL.Path = stripGatewayPrefix(req.URL.Path, instance.Slug, marker)
			if basePath != "" && !strings.HasPrefix(req.URL.Path, basePath) {
				req.URL.Path = basePath + req.URL.Path
			}
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
			req.URL.RawPath = ""
		})
		rp.ServeHTTP(c.Writer, c.Request)
	}
}

func (g *GatewayProxy) baseProxy(target *url.URL, director func(*http.Request)) *httputil.ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1
	rp.ErrorLog = log.New(io.Discard, "", 0)
	rp.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		scheme := "http"
		if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		req.Header.Set("X-Forwarded-Proto", scheme)
		director(req)
	}
	return rp
}

// authorizeKey validates the caller's portal API key against the instance.
// A global super-admin key (TenantID == nil) may access any instance;
// otherwise the key's tenant must match the instance's tenant and, when
// the key is instance-scoped, it must target this exact instance.
func (g *GatewayProxy) authorizeKey(c *gin.Context, plain string, instance *models.Instance) error {
	var key models.ApiKey
	if err := g.db.Where("key_hash = ?", security.HashAPIKey(plain)).First(&key).Error; err != nil {
		return fmt.Errorf("invalid API key")
	}
	if !key.Active {
		return fmt.Errorf("API key disabled")
	}
	nowVal := now()
	if key.ExpiresAt != nil && key.ExpiresAt.Before(nowVal) {
		return fmt.Errorf("API key expired")
	}
	// Global super-admin key: grants access to any instance.
	if key.TenantID != nil {
		if *key.TenantID != instance.TenantID {
			return fmt.Errorf("API key does not grant access to this instance")
		}
		if key.InstanceID != nil && *key.InstanceID != instance.ID {
			return fmt.Errorf("API key is scoped to another instance")
		}
	}
	g.db.Model(&models.ApiKey{}).Where("id = ?", key.ID).Update("last_used", &nowVal)
	return nil
}

func apiKeyFromRequest(c *gin.Context) string {
	if v := c.GetHeader("X-API-Key"); v != "" {
		return strings.TrimSpace(v)
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

// openAPIBase computes the instance's OpenAI-API base URL (ends with /v1).
func (g *GatewayProxy) openAPIBase(instance *models.Instance) string {
	if instance.Mode == models.ModeRemote && instance.OpenAPIURL != "" {
		return strings.TrimRight(instance.OpenAPIURL, "/")
	}
	name := instance.ContainerName
	if name == "" {
		name = fmt.Sprintf("hermes-inst-%d", instance.ID)
	}
	return fmt.Sprintf("http://%s:%d/v1", name, g.cfg.HermesOpenAPIPort)
}

// webhookBaseURL computes the base URL of an instance's channel webhook
// server. Local instances use the container name on the portal network;
// remote instances keep their declared scheme (https is preserved) and use
// the webhook port when the remote URL doesn't carry one.
func webhookBaseURL(instance *models.Instance, port int) string {
	if instance.Mode == models.ModeRemote {
		if u, err := url.Parse(instance.RemoteURL); err == nil {
			scheme := u.Scheme
			if scheme == "" {
				scheme = "http"
			}
			p := u.Port()
			if p == "" {
				p = strconv.Itoa(port)
			}
			return fmt.Sprintf("%s://%s:%s", scheme, u.Hostname(), p)
		}
	}
	name := instance.ContainerName
	if name == "" {
		name = fmt.Sprintf("hermes-inst-%d", instance.ID)
	}
	return fmt.Sprintf("http://%s:%d", name, port)
}

func stripGatewayPrefix(path, slug, marker string) string {
	prefix := "/api/v1/gateway/" + slug + marker
	if strings.HasPrefix(path, prefix) {
		rest := strings.TrimPrefix(path, prefix)
		if rest == "" {
			return "/"
		}
		return rest
	}
	return path
}

// WebhookTarget maps a channel name to the instance's (port, base path).
// Mirrors the defaults in hermes-agent's gateway platforms.
func WebhookTarget(channel string) (int, string) {
	switch channel {
	case "whatsapp", "whatsapp_cloud":
		return 8090, "/whatsapp/webhook"
	case "bluebubbles":
		return 8645, ""
	case "msgraph":
		return 8646, ""
	default: // webhook, weixin, qqbot, yuanbao share the generic webhook server
		return 8644, ""
	}
}
