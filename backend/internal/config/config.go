// Package config loads portal configuration from environment variables.
//
// 12-factor style: every knob is an env var, with sane defaults for local
// development. The same image can be deployed in dev / prod / air-gapped
// networks without code changes.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the resolved portal configuration.
type Config struct {
	AppName       string
	Debug         bool
	DataDir       string
	PublicBaseURL string // no trailing slash
	ListenAddr    string

	// JWT
	JWTSecret     string
	JWTAccessTTL  int // seconds
	JWTRefreshTTL int // seconds

	// Database
	DBPath string // default <DataDir>/portal.db

	// Docker (local instance management)
	DockerHost          string
	HermesImage         string
	HermesNetwork       string
	HermesUID           int
	HermesGID           int
	HermesDashboardPort int
	HermesOpenAPIPort   int

	// OIDC SSO
	OIDCEnabled       bool
	OIDCIssuer        string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCScopes        string
	OIDCAdminClaim    string
	OIDCAutoProvision bool

	// Bootstrap
	AdminUsername string
	AdminPassword string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}

// Load reads configuration from the environment.
func Load() *Config {
	c := &Config{
		AppName:             env("PORTAL_APP_NAME", "Hermes Portal"),
		Debug:               envBool("PORTAL_DEBUG", false),
		DataDir:             env("PORTAL_DATA_DIR", "/app/data"),
		PublicBaseURL:       strings.TrimRight(env("PORTAL_PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		ListenAddr:          env("PORTAL_LISTEN_ADDR", "0.0.0.0:8080"),
		JWTSecret:           env("PORTAL_JWT_SECRET", "change-me-in-production"),
		JWTAccessTTL:        envInt("PORTAL_JWT_ACCESS_TTL", 3600),
		JWTRefreshTTL:       envInt("PORTAL_JWT_REFRESH_TTL", 30*86400),
		DBPath:              env("PORTAL_DB_PATH", ""),
		DockerHost:          env("DOCKER_HOST", "unix:///var/run/docker.sock"),
		HermesImage:         env("PORTAL_HERMES_IMAGE", "hermes-agent"),
		HermesNetwork:       env("PORTAL_HERMES_NETWORK", "hermes-portal-net"),
		HermesUID:           envInt("PORTAL_HERMES_UID", 1000),
		HermesGID:           envInt("PORTAL_HERMES_GID", 1000),
		HermesDashboardPort: envInt("PORTAL_HERMES_DASHBOARD_PORT", 9119),
		HermesOpenAPIPort:   envInt("PORTAL_HERMES_OPENAPI_PORT", 8642),
		OIDCEnabled:         envBool("PORTAL_OIDC_ENABLED", false),
		OIDCIssuer:          env("PORTAL_OIDC_ISSUER", ""),
		OIDCClientID:        env("PORTAL_OIDC_CLIENT_ID", ""),
		OIDCClientSecret:    env("PORTAL_OIDC_CLIENT_SECRET", ""),
		OIDCScopes:          env("PORTAL_OIDC_SCOPES", "openid profile email"),
		OIDCAdminClaim:      env("PORTAL_OIDC_ADMIN_CLAIM", "hermes_portal_admin"),
		OIDCAutoProvision:   envBool("PORTAL_OIDC_AUTO_PROVISION", false),
		AdminUsername:       env("PORTAL_ADMIN_USERNAME", "admin"),
		AdminPassword:       env("PORTAL_ADMIN_PASSWORD", ""),
	}
	if c.DBPath == "" {
		c.DBPath = c.DataDir + "/portal.db"
	}
	return c
}
