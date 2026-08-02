package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"hermesportal/internal/config"
	"hermesportal/internal/models"
	"hermesportal/internal/security"
)

// InstanceService orchestrates instance lifecycle + health.
type InstanceService struct {
	cfg    *config.Config
	db     *gorm.DB
	docker *DockerClient
	cache  *DashboardSessionCache
}

// NewInstanceService wires the service.
func NewInstanceService(cfg *config.Config, db *gorm.DB, docker *DockerClient, cache *DashboardSessionCache) *InstanceService {
	return &InstanceService{cfg: cfg, db: db, docker: docker, cache: cache}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a name into a URL-safe slug.
func Slugify(name, base string) string {
	s := strings.TrimSpace(name)
	if base != "" {
		s = strings.TrimSpace(base)
	}
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "instance"
	}
	return s
}

// GenerateInstanceConfig creates fresh secrets for a new instance.
func GenerateInstanceConfig(name string) (models.InstanceConfig, error) {
	apiKey, err := security.RandomSecret(32)
	if err != nil {
		return models.InstanceConfig{}, err
	}
	pass, err := security.RandomSecret(16)
	if err != nil {
		return models.InstanceConfig{}, err
	}
	secret, err := security.RandomSecret(32)
	if err != nil {
		return models.InstanceConfig{}, err
	}
	return models.InstanceConfig{
		APIServerKey:    apiKey,
		DashboardUser:   fmt.Sprintf("portal-%s", truncateName(name)),
		DashboardPass:   pass,
		DashboardSecret: secret,
		ExtraEnv:        map[string]string{},
	}, nil
}

func truncateName(name string) string {
	clean := strings.ToLower(slugRe.ReplaceAllString(name, "-"))
	if len(clean) > 16 {
		clean = clean[:16]
	}
	return strings.Trim(clean, "-")
}

// ContainerName returns the docker container name for a local instance.
func ContainerName(instanceID uint) string {
	return fmt.Sprintf("hermes-inst-%d", instanceID)
}

// VolumeName returns the named data volume for a local instance.
func VolumeName(instanceID uint) string {
	return fmt.Sprintf("hermes-inst-%d-data", instanceID)
}

// Create creates a new instance row and (for docker mode) its container.
func (s *InstanceService) Create(ctx context.Context, tenantID uint, name, mode, slug, image, remoteURL, openAPIURL string, modelID *uint, defaultModel *models.DefaultModel, extraEnv map[string]string, createdBy *uint) (*models.Instance, error) {
	slug = Slugify(name, slug)
	var count int64
	s.db.Model(&models.Instance{}).Where("slug = ?", slug).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("instance slug '%s' already exists", slug)
	}

	if mode == models.ModeDocker {
		if err := s.docker.Ping(ctx); err != nil {
			return nil, fmt.Errorf("docker is not reachable from the portal container: %w", err)
		}
	} else if mode == models.ModeRemote {
		if !strings.HasPrefix(remoteURL, "http://") && !strings.HasPrefix(remoteURL, "https://") {
			return nil, fmt.Errorf("remote_url must be a valid http(s) URL")
		}
		if openAPIURL == "" {
			openAPIURL = strings.TrimRight(remoteURL, "/") + "/v1"
		}
	} else {
		return nil, fmt.Errorf("unknown mode %q", mode)
	}

	cfg, err := GenerateInstanceConfig(name)
	if err != nil {
		return nil, err
	}
	cfg.ExtraEnv = extraEnv
	cfg.DefaultModel = defaultModel

	inst := &models.Instance{
		TenantID:   tenantID,
		Name:       name,
		Slug:       slug,
		Mode:       mode,
		Image:      image,
		Status:     models.StatusCreated,
		Config:     security.MarshalJSON(cfg),
		RemoteURL:  remoteURL,
		OpenAPIURL: openAPIURL,
		ModelID:    modelID,
		CreatedBy:  createdBy,
	}
	if err := s.db.Create(inst).Error; err != nil {
		return nil, err
	}

	if mode == models.ModeDocker {
		if err := s.startContainer(ctx, inst, cfg); err != nil {
			// Keep the row for diagnostics but surface the failure.
			inst.Status = models.StatusError
			s.db.Model(inst).Update("status", models.StatusError)
			return nil, fmt.Errorf("container start failed: %w", err)
		}
		inst.Status = models.StatusStarting
		inst.ContainerName = ContainerName(inst.ID)
		s.db.Model(inst).Updates(map[string]any{"status": models.StatusStarting, "container_name": inst.ContainerName})
		// Asynchronously probe readiness so the status transitions
		// starting → running without requiring a page refresh, then
		// assign the configured default model via hermes' own API.
		go s.waitReady(context.Background(), inst, cfg)
	} else {
		inst.Status = models.StatusRunning
		s.db.Model(inst).Update("status", models.StatusRunning)
	}
	return inst, nil
}

// waitReady polls the instance dashboard until it becomes healthy, then
// lets Health() persist the running status and assigns the configured
// default model. Gives up silently after a timeout — the page-level
// polling keeps trying afterwards (model assignment retried on edit).
func (s *InstanceService) waitReady(ctx context.Context, inst *models.Instance, cfg models.InstanceConfig) {
	deadline := time.Now().Add(180 * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if time.Now().After(deadline) {
			log.Printf("[portal] instance %d readiness probe timed out (status stays %s)",
				inst.ID, models.StatusStarting)
			return
		}

		var fresh models.Instance
		if err := s.db.First(&fresh, inst.ID).Error; err != nil {
			return
		}
		// User stopped / destroyed it meanwhile.
		if fresh.Status == models.StatusStopped || fresh.Status == models.StatusDestroyed {
			return
		}
		result := s.Health(ctx, &fresh)
		if ok, _ := result["ok"].(bool); ok {
			// Health() already persisted status=running — now wire the
			// default model through hermes' native API.
			if err := s.configureDefaultModel(ctx, &fresh, cfg); err != nil {
				log.Printf("[portal] instance %d default-model assignment failed: %v",
					inst.ID, err)
			} else {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// configureDefaultModel assigns the instance's default model by calling
// hermes' own POST /api/model/set (writes model.provider / model.default /
// model.base_url / model.api_key into the instance config.yaml). It uses
// the portal-held dashboard session, so no hermes code changes are needed.
func (s *InstanceService) configureDefaultModel(ctx context.Context, inst *models.Instance, cfg models.InstanceConfig) error {
	dm := cfg.DefaultModel
	if dm == nil || strings.TrimSpace(dm.Model) == "" || strings.TrimSpace(dm.URL) == "" {
		return nil // nothing to configure
	}
	provider := strings.TrimSpace(dm.Provider)
	if provider == "" {
		provider = "custom"
	}
	payload, _ := json.Marshal(map[string]any{
		"scope":                   "main",
		"provider":                provider,
		"model":                   strings.TrimSpace(dm.Model),
		"base_url":                strings.TrimSpace(dm.URL),
		"api_key":                 strings.TrimSpace(dm.Key),
		"confirm_expensive_model": true,
	})
	base := InstanceDashboardURL(s.cache, inst)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/model/set", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Prefix", Prefix(inst.ID))
	if cookie := s.cache.CookieHeader(ctx, inst); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("model/set returned %d: %s", resp.StatusCode, truncateString(string(body), 512))
	}
	log.Printf("[portal] instance %d default model configured: provider=%s model=%s",
		inst.ID, provider, dm.Model)
	return nil
}

func (s *InstanceService) startContainer(ctx context.Context, inst *models.Instance, cfg models.InstanceConfig) error {
	if err := s.docker.EnsureNetwork(ctx, s.cfg.HermesNetwork); err != nil {
		return err
	}
	if err := s.docker.EnsureVolume(ctx, VolumeName(inst.ID)); err != nil {
		return err
	}
	inst.ContainerName = ContainerName(inst.ID)
	env := s.containerEnv(inst, cfg)
	spec := ContainerSpec{
		Name:          inst.ContainerName,
		Image:         inst.Image,
		Command:       []string{"gateway", "run"},
		Env:           env,
		Network:       s.cfg.HermesNetwork,
		VolumeMount:   VolumeName(inst.ID) + ":/opt/data",
		RestartPolicy: "unless-stopped",
		Labels: map[string]string{
			"hermes.portal":          "1",
			"hermes.portal.instance": fmt.Sprintf("%d", inst.ID),
		},
		MemLimit: cfg.MemLimit,
	}
	if spec.Image == "" {
		spec.Image = s.cfg.HermesImage
	}
	_, err := s.docker.CreateContainer(ctx, spec)
	return err
}

func (s *InstanceService) containerEnv(inst *models.Instance, cfg models.InstanceConfig) []string {
	env := map[string]string{
		"HERMES_UID":                           fmt.Sprintf("%d", s.cfg.HermesUID),
		"HERMES_GID":                           fmt.Sprintf("%d", s.cfg.HermesGID),
		"HERMES_DASHBOARD":                     "1",
		"HERMES_DASHBOARD_HOST":                "0.0.0.0",
		"HERMES_DASHBOARD_PORT":                fmt.Sprintf("%d", s.cfg.HermesDashboardPort),
		"HERMES_DASHBOARD_BASIC_AUTH_USERNAME": cfg.DashboardUser,
		"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD": cfg.DashboardPass,
		"HERMES_DASHBOARD_BASIC_AUTH_SECRET":   cfg.DashboardSecret,
		"API_SERVER_HOST":                      "0.0.0.0",
		"API_SERVER_PORT":                      fmt.Sprintf("%d", s.cfg.HermesOpenAPIPort),
		"API_SERVER_KEY":                       cfg.APIServerKey,
	}
	for k, v := range cfg.ExtraEnv {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// Start / Stop / Restart apply only to docker-mode instances.
func (s *InstanceService) Start(ctx context.Context, inst *models.Instance) error {
	if inst.Mode != models.ModeDocker {
		return errors.New("remote instances are managed outside the portal; start is not applicable")
	}
	if err := s.docker.ContainerAction(ctx, inst.ContainerName, "start"); err != nil {
		return err
	}
	s.db.Model(inst).Updates(map[string]any{"status": models.StatusStarting, "updated_at": time.Now()})
	s.cache.Invalidate(inst.ID)
	return nil
}

func (s *InstanceService) Stop(ctx context.Context, inst *models.Instance) error {
	if inst.Mode != models.ModeDocker {
		return errors.New("remote instances are managed outside the portal; stop is not applicable")
	}
	if err := s.docker.ContainerAction(ctx, inst.ContainerName, "stop"); err != nil {
		return err
	}
	s.db.Model(inst).Updates(map[string]any{"status": models.StatusStopped, "updated_at": time.Now()})
	s.cache.Invalidate(inst.ID)
	return nil
}

func (s *InstanceService) Restart(ctx context.Context, inst *models.Instance) error {
	if inst.Mode != models.ModeDocker {
		return errors.New("remote instances are managed outside the portal; restart is not applicable")
	}
	if err := s.docker.ContainerAction(ctx, inst.ContainerName, "restart"); err != nil {
		return err
	}
	s.db.Model(inst).Updates(map[string]any{"status": models.StatusStarting, "updated_at": time.Now()})
	s.cache.Invalidate(inst.ID)
	return nil
}

// Recreate destroys and recreates the container (used by instance update).
func (s *InstanceService) Recreate(ctx context.Context, inst *models.Instance) error {
	var cfg models.InstanceConfig
	_ = security.UnmarshalJSON(inst.Config, &cfg)
	s.docker.RemoveContainer(ctx, inst.ContainerName)
	if err := s.startContainer(ctx, inst, cfg); err != nil {
		inst.Status = models.StatusError
		s.db.Model(inst).Update("status", models.StatusError)
		return err
	}
	inst.Status = models.StatusStarting
	s.db.Model(inst).Updates(map[string]any{"status": models.StatusStarting, "updated_at": time.Now()})
	s.cache.Invalidate(inst.ID)
	go s.waitReady(context.Background(), inst, cfg)
	return nil
}

// Destroy removes the container and (optionally) its data volume.
func (s *InstanceService) Destroy(ctx context.Context, inst *models.Instance, keepVolume bool) {
	if inst.Mode == models.ModeDocker {
		s.docker.RemoveContainer(ctx, inst.ContainerName)
		if !keepVolume {
			if err := s.docker.RemoveVolume(ctx, VolumeName(inst.ID)); err != nil {
				log.Printf("[portal] volume removal %s: %v", VolumeName(inst.ID), err)
			}
		}
	}
	inst.Status = models.StatusDestroyed
	s.db.Model(inst).Updates(map[string]any{"status": models.StatusDestroyed, "updated_at": time.Now()})
	s.cache.Invalidate(inst.ID)
}

// Health probes the instance dashboard's /api/health endpoint.
func (s *InstanceService) Health(ctx context.Context, inst *models.Instance) map[string]any {
	base := InstanceDashboardURL(s.cache, inst)
	url := strings.TrimRight(base, "/") + "/api/health"
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get(url)
	now := time.Now()
	result := map[string]any{"ok": false}
	if err != nil {
		result["error"] = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			result["ok"] = true
			inst.Status = models.StatusRunning
			inst.LastHeartbeat = &now
			s.db.Model(inst).Updates(map[string]any{"status": models.StatusRunning, "last_heartbeat": &now})
		} else {
			result["status"] = resp.StatusCode
		}
	}
	if inst.Mode == models.ModeDocker {
		result["container_state"] = s.docker.ContainerState(ctx, inst.ContainerName)
		if !result["ok"].(bool) && (result["container_state"] == "" || result["container_state"] == "stopped") {
			inst.Status = models.StatusStopped
			inst.LastHeartbeat = &now
		}
	}
	s.db.Model(inst).Updates(map[string]any{"status": inst.Status, "last_heartbeat": inst.LastHeartbeat})
	result["status"] = inst.Status
	return result
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
