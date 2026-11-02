package github

import "testing"

func TestConfiguredPublicGitHubAppValues(t *testing.T) {
	t.Setenv("AETHER_GITHUB_CLIENT_ID", "")
	t.Setenv("AETHER_GITHUB_APP_SLUG", "")
	oldID, oldSlug := EmbeddedClientID, EmbeddedAppSlug
	EmbeddedClientID, EmbeddedAppSlug = "release-client-id", "aether-dots"
	t.Cleanup(func() {
		EmbeddedClientID, EmbeddedAppSlug = oldID, oldSlug
	})
	if ConfiguredClientID() != "release-client-id" || ConfiguredAppSlug() != "aether-dots" {
		t.Fatal("release configuration was not used when environment overrides were empty")
	}
	t.Setenv("AETHER_GITHUB_CLIENT_ID", " environment-client-id ")
	t.Setenv("AETHER_GITHUB_APP_SLUG", " user-app-slug ")
	if ConfiguredClientID() != "environment-client-id" || ConfiguredAppSlug() != "user-app-slug" {
		t.Fatal("explicit environment overrides were not trimmed or prioritized")
	}
}
