package proxy

import (
	"testing"

	"hermesportal/internal/models"
	"hermesportal/internal/services"
)

func TestSlugify(t *testing.T) {
	cases := []struct{ name, base, want string }{
		{"My Instance", "", "my-instance"},
		{"Demo 123!", "", "demo-123"},
		{"", "Already-Slug", "already-slug"},
		{"  Spaces  ", "", "spaces"},
		{"!!", "", "instance"}, // empty fallback
	}
	for _, c := range cases {
		got := services.Slugify(c.name, c.base)
		if got != c.want {
			t.Errorf("Slugify(%q,%q)=%q want %q", c.name, c.base, got, c.want)
		}
	}
}

func TestUniqueFallbackSlug(t *testing.T) {
	// Deterministic slugs pass through untouched.
	if got := services.UniqueFallbackSlug("my-instance"); got != "my-instance" {
		t.Fatalf("deterministic slug changed: %q", got)
	}
	// The generic fallback gets a unique suffix so non-ASCII names can coexist.
	a := services.UniqueFallbackSlug("instance")
	b := services.UniqueFallbackSlug("instance")
	if a == "instance" || b == "instance" || a == b {
		t.Fatalf("generic slugs must be unique: %q %q", a, b)
	}
}

func TestRedactExtraEnv(t *testing.T) {
	in := map[string]string{
		"DEEPSEEK_API_KEY":                     "sk-secret",
		"OPENROUTER_API_KEY":                   "or-secret",
		"FOO_TOKEN":                            "tok",
		"MY_PASSWORD":                          "pw",
		"LOG_LEVEL":                            "debug",
		"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD": "dashboard-pw",
	}
	out := services.RedactExtraEnv(in)
	for _, k := range []string{"DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "FOO_TOKEN", "MY_PASSWORD", "HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"} {
		if out[k] != services.RedactedSecret {
			t.Fatalf("secret %q not redacted: %q", k, out[k])
		}
	}
	if out["LOG_LEVEL"] != "debug" {
		t.Fatalf("non-secret redacted: %q", out["LOG_LEVEL"])
	}
	if services.RedactExtraEnv(nil) != nil {
		t.Fatal("nil env must stay nil")
	}
}

func TestMergeExtraEnv(t *testing.T) {
	current := map[string]string{"DEEPSEEK_API_KEY": "real-key", "LOG_LEVEL": "info"}
	// Echoing the redaction placeholder preserves the stored secret.
	merged := services.MergeExtraEnv(current, map[string]string{
		"DEEPSEEK_API_KEY": services.RedactedSecret,
		"LOG_LEVEL":        "debug",
	})
	if merged["DEEPSEEK_API_KEY"] != "real-key" {
		t.Fatalf("secret lost: %q", merged["DEEPSEEK_API_KEY"])
	}
	if merged["LOG_LEVEL"] != "debug" {
		t.Fatalf("non-secret not updated: %q", merged["LOG_LEVEL"])
	}
	// Omitted keys are dropped (full-map replace semantics).
	merged2 := services.MergeExtraEnv(current, map[string]string{})
	if _, ok := merged2["LOG_LEVEL"]; ok {
		t.Fatal("omitted key must be dropped")
	}
}

func TestApplyAndRemoveProviderEnv(t *testing.T) {
	cfg := &models.InstanceConfig{DefaultModel: &models.DefaultModel{Provider: "deepseek", Key: "sk-1"}}
	services.ApplyProviderEnv(cfg)
	if cfg.ExtraEnv["DEEPSEEK_API_KEY"] != "sk-1" {
		t.Fatalf("provider env not injected: %v", cfg.ExtraEnv)
	}
	// Key rotation must overwrite the existing env value.
	cfg.DefaultModel.Key = "sk-2"
	services.ApplyProviderEnv(cfg)
	if cfg.ExtraEnv["DEEPSEEK_API_KEY"] != "sk-2" {
		t.Fatalf("key rotation not propagated: %v", cfg.ExtraEnv)
	}
	// Clearing the model removes the injected env var.
	services.RemoveProviderEnv(cfg, cfg.DefaultModel)
	if _, ok := cfg.ExtraEnv["DEEPSEEK_API_KEY"]; ok {
		t.Fatalf("provider env not removed on clear: %v", cfg.ExtraEnv)
	}
}

func TestWebhookBaseURL(t *testing.T) {
	local := &models.Instance{ID: 3, Mode: models.ModeDocker, ContainerName: "hermes-inst-3"}
	if got := webhookBaseURL(local, 8644); got != "http://hermes-inst-3:8644" {
		t.Fatalf("local webhook base: %q", got)
	}
	remote := &models.Instance{Mode: models.ModeRemote, RemoteURL: "https://hermes.example.com"}
	if got := webhookBaseURL(remote, 8644); got != "https://hermes.example.com:8644" {
		t.Fatalf("remote webhook base (scheme+port): %q", got)
	}
	remoteWithPort := &models.Instance{Mode: models.ModeRemote, RemoteURL: "https://hermes.example.com:9443"}
	if got := webhookBaseURL(remoteWithPort, 8644); got != "https://hermes.example.com:9443" {
		t.Fatalf("remote webhook base keeps explicit port: %q", got)
	}
}

func TestContainerNaming(t *testing.T) {
	if services.ContainerName(5) != "hermes-inst-5" {
		t.Fatal("ContainerName mismatch")
	}
	if services.VolumeName(5) != "hermes-inst-5-data" {
		t.Fatal("VolumeName mismatch")
	}
}

func TestWebhookTarget(t *testing.T) {
	cases := []struct {
		channel  string
		wantPort int
		wantPath string
	}{
		{"whatsapp", 8090, "/whatsapp/webhook"},
		{"whatsapp_cloud", 8090, "/whatsapp/webhook"},
		{"webhook", 8644, ""},
		{"bluebubbles", 8645, ""},
		{"msgraph", 8646, ""},
	}
	for _, c := range cases {
		port, path := WebhookTarget(c.channel)
		if port != c.wantPort || path != c.wantPath {
			t.Errorf("WebhookTarget(%q) = (%d,%q) want (%d,%q)",
				c.channel, port, path, c.wantPort, c.wantPath)
		}
	}
}

// TestStripGatewayPrefix verifies the /openapi passthrough keeps the
// instance's native paths: cron REST (/api/jobs), sessions, runs and the
// OpenAI surface (/v1/...).
func TestStripGatewayPrefix(t *testing.T) {
	cases := []struct {
		path, slug, marker, want string
	}{
		{"/api/v1/gateway/test1/openapi/v1/models", "test1", "/openapi", "/v1/models"},
		{"/api/v1/gateway/test1/openapi/v1/chat/completions", "test1", "/openapi", "/v1/chat/completions"},
		{"/api/v1/gateway/test1/openapi/api/jobs", "test1", "/openapi", "/api/jobs"},
		{"/api/v1/gateway/test1/openapi/api/jobs/42/pause", "test1", "/openapi", "/api/jobs/42/pause"},
		{"/api/v1/gateway/test1/openapi/api/sessions", "test1", "/openapi", "/api/sessions"},
		{"/api/v1/gateway/test1/openapi/v1/runs/abc/stop", "test1", "/openapi", "/v1/runs/abc/stop"},
	}
	for _, c := range cases {
		got := stripGatewayPrefix(c.path, c.slug, c.marker)
		if got != c.want {
			t.Errorf("stripGatewayPrefix(%q) = %q want %q", c.path, got, c.want)
		}
	}
}
