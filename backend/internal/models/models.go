// Package models defines the portal data model (GORM).
//
// Persistence is SQLite (the same engine Hermes Agent uses for its state),
// via the pure-Go glebarez/sqlite driver — no CGO required.
package models

import (
	"time"
)

// Roles.
const (
	RoleSuperAdmin  = "super_admin"
	RoleTenantAdmin = "tenant_admin"
	RoleMember      = "member"
)

// Instance modes.
const (
	ModeDocker = "docker"
	ModeRemote = "remote"
)

// Instance statuses.
const (
	StatusCreated   = "created"
	StatusStarting  = "starting"
	StatusRunning   = "running"
	StatusStopped   = "stopped"
	StatusError     = "error"
	StatusDestroyed = "destroyed"
)

// Tenant is the top-level isolation boundary. Every instance, user and
// API key belongs to exactly one tenant.
type Tenant struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"size:128;not null"`
	Slug        string `gorm:"size:64;uniqueIndex;not null"`
	Description string `gorm:"size:512"`
	Settings    string `gorm:"type:text"` // JSON
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// User is a portal account. Roles: super_admin (no tenant), tenant_admin,
// member (tenant-scoped). OIDC users carry their (issuer, subject) link.
type User struct {
	ID           uint   `gorm:"primaryKey"`
	TenantID     *uint  `gorm:"index"`
	Username     string `gorm:"size:128;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:512"`
	Email        string `gorm:"size:256"`
	Role         string `gorm:"size:32;not null;default:member"`
	OIDCSub      string `gorm:"size:256;index"`
	OIDCIssuer   string `gorm:"size:512"`
	Active       bool   `gorm:"default:true"`
	LastLogin    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DefaultModel is the default inference provider/model assigned to an
// instance via hermes' own POST /api/model/set (model.provider / model.
// default / model.base_url / model.api_key in its config.yaml).
type DefaultModel struct {
	URL      string `json:"url"`
	Model    string `json:"model"`
	Key      string `json:"key,omitempty"`
	Provider string `json:"provider,omitempty"` // default: custom
}

// InstanceConfig is the JSON blob stored in Instance.Config.
type InstanceConfig struct {
	APIServerKey    string            `json:"api_server_key"`
	DashboardUser   string            `json:"dashboard_username"`
	DashboardPass   string            `json:"dashboard_password"`
	DashboardSecret string            `json:"dashboard_secret"`
	ExtraEnv        map[string]string `json:"extra_env,omitempty"`
	MemLimit        string            `json:"mem_limit,omitempty"`
	VolumeName      string            `json:"volume,omitempty"`
	DefaultModel    *DefaultModel     `json:"default_model,omitempty"`
}

// ModelConfig is a reusable default-model template (the model library).
// Tenants maintain their own set; the default one is offered when
// creating instances. The Key is stored here so instance creation can
// snapshot it into the instance's own config.
type ModelConfig struct {
	ID        uint   `gorm:"primaryKey"`
	TenantID  uint   `gorm:"index;not null"`
	Name      string `gorm:"size:128;not null"` // display name, e.g. "OpenAI GPT-4o"
	Slug      string `gorm:"size:64"`
	Provider  string `gorm:"size:64"`           // custom | openrouter | openai | ...
	URL       string `gorm:"size:512;not null"` // OpenAI-compatible endpoint base URL
	Model     string `gorm:"size:256;not null"` // model name on the endpoint
	Key       string `gorm:"size:512"`          // API key (kept out of API responses)
	IsDefault bool   `gorm:"default:false;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Instance is a hermes-agent deployment — either a local container
// (mode=docker) or an onboarded remote endpoint (mode=remote).
type Instance struct {
	ID            uint   `gorm:"primaryKey"`
	TenantID      uint   `gorm:"index;not null"`
	Name          string `gorm:"size:128;not null"`
	Slug          string `gorm:"size:64;uniqueIndex;not null"`
	Mode          string `gorm:"size:16;not null;default:docker"`
	Image         string `gorm:"size:256"`
	ContainerName string `gorm:"size:128"`
	ModelID       *uint  `gorm:"index"` // optional reference to a ModelConfig
	Status        string `gorm:"size:32;not null;default:created"`
	Config        string `gorm:"type:text"` // JSON InstanceConfig
	RemoteURL     string `gorm:"size:512"`
	// OpenAPIURL overrides the derived OpenAI-API base URL (remote mode).
	// Example: https://hermes-remote.example.com/v1
	OpenAPIURL    string `gorm:"size:512"`
	LastHeartbeat *time.Time
	CreatedBy     *uint
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ApiKey is a portal-issued credential for the unified OpenAI gateway.
// Only the SHA-256 hash is stored; the plaintext prefix is kept for UI
// display and lookup efficiency.
type ApiKey struct {
	ID         uint   `gorm:"primaryKey"`
	TenantID   uint   `gorm:"index;not null"`
	InstanceID *uint  `gorm:"index"` // NULL = tenant-wide
	Name       string `gorm:"size:128;not null"`
	KeyPrefix  string `gorm:"size:16;not null"`
	KeyHash    string `gorm:"size:64;uniqueIndex;not null"`
	Scopes     string `gorm:"size:256;not null;default:[\"openapi\"]"`
	Active     bool   `gorm:"default:true"`
	ExpiresAt  *time.Time
	LastUsed   *time.Time
	CreatedBy  *uint
	CreatedAt  time.Time
}

// AuditLog records privileged operations for the operator.
type AuditLog struct {
	ID        uint  `gorm:"primaryKey"`
	TenantID  *uint `gorm:"index"`
	ActorID   *uint
	Action    string    `gorm:"size:64;not null"`
	Target    string    `gorm:"size:256"`
	Detail    string    `gorm:"type:text"`
	IP        string    `gorm:"size:64"`
	CreatedAt time.Time `gorm:"index"`
}
