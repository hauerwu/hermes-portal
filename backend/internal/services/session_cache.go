package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

// DashboardSessionCache holds per-instance dashboard session cookies.
//
// The portal is the only holder of each instance's dashboard session: it
// logs in with the generated basic-auth credentials through the same
// “/auth/password-login“ endpoint the dashboard's own SPA uses, then
// injects the cookies into every proxied request (HTTP and WebSocket).
// Set-Cookie headers seen on proxied responses are captured back into the
// cache so token refreshes by the instance stay transparent.
type DashboardSessionCache struct {
	cfg *config.Config
	db  *gorm.DB

	mu      sync.Mutex
	entries map[uint]*sessionEntry
}

type sessionEntry struct {
	cookies   map[string]string
	expiresAt time.Time
}

// NewDashboardSessionCache builds the cache.
func NewDashboardSessionCache(cfg *config.Config, db *gorm.DB) *DashboardSessionCache {
	return &DashboardSessionCache{
		cfg:     cfg,
		db:      db,
		entries: map[uint]*sessionEntry{},
	}
}

// Prefix returns the URL prefix the instance dashboard is served under.
func Prefix(instanceID uint) string {
	return fmt.Sprintf("/instances/%d/dashboard", instanceID)
}

// CookieHeader returns a valid “Cookie“ header value for the instance,
// bootstrapping a session if none is cached. Returns "" when the session
// could not be established (instance down / not configured).
func (c *DashboardSessionCache) CookieHeader(ctx context.Context, instance *models.Instance) string {
	c.mu.Lock()
	entry, ok := c.entries[instance.ID]
	c.mu.Unlock()
	if ok && entry.expiresAt.After(time.Now()) && len(entry.cookies) > 0 {
		return cookify(entry.cookies)
	}

	// Bootstrap (single-flight per instance).
	lock := instanceLock(instance.ID)
	lock.Lock()
	defer lock.Unlock()

	// Re-check under the lock.
	c.mu.Lock()
	entry, ok = c.entries[instance.ID]
	c.mu.Unlock()
	if ok && entry.expiresAt.After(time.Now()) && len(entry.cookies) > 0 {
		return cookify(entry.cookies)
	}

	// Remote instances hold credentials the portal does not know; a portal-
	// generated basic-auth bootstrap would always fail. Their dashboard
	// sessions are established through the embedded login form (captured back
	// into this cache via Capture) instead.
	if instance.Mode == models.ModeRemote {
		return ""
	}

	cookies, err := c.bootstrap(ctx, instance)
	if err != nil {
		log.Printf("[portal] session bootstrap failed for instance %d: %v", instance.ID, err)
		return ""
	}
	c.mu.Lock()
	c.entries[instance.ID] = &sessionEntry{
		cookies:   cookies,
		expiresAt: time.Now().Add(12 * time.Hour), // instance access TTL; refresh captured below
	}
	c.mu.Unlock()
	return cookify(cookies)
}

// Invalidate drops a cached session (forces re-login on next request).
func (c *DashboardSessionCache) Invalidate(instanceID uint) {
	c.mu.Lock()
	delete(c.entries, instanceID)
	c.mu.Unlock()
}

// Capture merges Set-Cookie headers from a proxied response into the cache.
func (c *DashboardSessionCache) Capture(instanceID uint, setCookieHeaders []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[instanceID]
	if entry == nil {
		return
	}
	for _, header := range setCookieHeaders {
		pair := strings.SplitN(header, ";", 2)[0]
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		v = strings.Trim(v, `"`)
		if v == "" {
			delete(entry.cookies, k)
		} else {
			entry.cookies[k] = v
		}
	}
}

func (c *DashboardSessionCache) bootstrap(ctx context.Context, instance *models.Instance) (map[string]string, error) {
	var cfg models.InstanceConfig
	if err := security.UnmarshalJSON(instance.Config, &cfg); err != nil {
		return nil, fmt.Errorf("bad instance config: %w", err)
	}
	if cfg.DashboardUser == "" || cfg.DashboardPass == "" {
		return nil, fmt.Errorf("instance %d has no dashboard credentials", instance.ID)
	}
	base := InstanceDashboardURL(c, instance)
	body, _ := json.Marshal(map[string]string{
		"provider": "basic",
		"username": cfg.DashboardUser,
		"password": cfg.DashboardPass,
		"next":     "",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/auth/password-login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Prefix", Prefix(instance.ID))

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 && resp.StatusCode != 302 {
		return nil, fmt.Errorf("dashboard login failed: %d", resp.StatusCode)
	}
	cookies := map[string]string{}
	for _, cookie := range resp.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	for _, header := range resp.Header.Values("Set-Cookie") {
		kv := strings.SplitN(header, "=", 2)
		if len(kv) == 2 {
			name := strings.TrimSpace(kv[0])
			value := strings.Split(kv[1], ";")[0]
			cookies[name] = strings.Trim(value, `"`)
		}
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("dashboard login returned no session cookies")
	}
	return cookies, nil
}

// instanceLocks provides a per-instance mutex map for single-flight logins.
var instanceLocks struct {
	mu    sync.Mutex
	locks map[uint]*sync.Mutex
}

func instanceLock(id uint) *sync.Mutex {
	instanceLocks.mu.Lock()
	defer instanceLocks.mu.Unlock()
	if instanceLocks.locks == nil {
		instanceLocks.locks = map[uint]*sync.Mutex{}
	}
	if m, ok := instanceLocks.locks[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	instanceLocks.locks[id] = m
	return m
}

func cookify(cookies map[string]string) string {
	parts := make([]string, 0, len(cookies))
	for k, v := range cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// InstanceDashboardURL computes the internal dashboard URL for an instance.
// Local instances are reached by container name on the portal network;
// remote instances by their onboarded URL.
func InstanceDashboardURL(c *DashboardSessionCache, instance *models.Instance) string {
	if instance.Mode == models.ModeRemote {
		return strings.TrimRight(instance.RemoteURL, "/")
	}
	name := instance.ContainerName
	if name == "" {
		name = fmt.Sprintf("hermes-inst-%d", instance.ID)
	}
	return fmt.Sprintf("http://%s:%d", name, c.cfg.HermesDashboardPort)
}
