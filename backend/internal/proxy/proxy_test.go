package proxy

import (
	"testing"

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
