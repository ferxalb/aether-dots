package github

import (
	"os"
	"strings"
)

// EmbeddedClientID and EmbeddedAppSlug are public configuration for release
// builds. Set them with linker flags; neither value is a secret.
var EmbeddedClientID string
var EmbeddedAppSlug string

// ConfiguredClientID uses the explicit environment override when present and
// otherwise returns the public ID embedded by a release build.
func ConfiguredClientID() string {
	if value := strings.TrimSpace(os.Getenv("AETHER_GITHUB_CLIENT_ID")); value != "" {
		return value
	}
	return strings.TrimSpace(EmbeddedClientID)
}

func ConfiguredAppSlug() string {
	if value := strings.TrimSpace(os.Getenv("AETHER_GITHUB_APP_SLUG")); value != "" {
		return value
	}
	return strings.TrimSpace(EmbeddedAppSlug)
}
