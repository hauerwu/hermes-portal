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
