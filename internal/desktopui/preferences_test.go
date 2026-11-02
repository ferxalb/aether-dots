package desktopui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesAddModelControlsWithoutBreakingOlderFiles(t *testing.T) {
	for _, name := range []string{"AETHER_REASONING_EFFORT", "AETHER_TEXT_VERBOSITY", "AETHER_MAX_OUTPUT_TOKENS", "AETHER_MAX_RETRIES", "AETHER_MODEL_TIMEOUT_SECONDS"} {
		t.Setenv(name, "")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, preferencesFile), []byte(`{"model":"gpt-custom","appearance":"Dark","palette":"forest"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prefs, err := loadPreferences(dir)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Model != "gpt-custom" || prefs.ReasoningEffort != "medium" || prefs.TextVerbosity != "medium" ||
		prefs.MaxOutputTokens != "" || prefs.MaxOutputTokensSet || prefs.MaxRetries != "2" || prefs.TimeoutSeconds != "90" {
		t.Fatalf("older preferences did not receive generation defaults: %#v", prefs)
	}
	prefs.ReasoningEffort = "high"
	prefs.TextVerbosity = "low"
	prefs.MaxOutputTokens = "16384"
	prefs.MaxOutputTokensSet = true
	prefs.MaxRetries = "0"
	prefs.TimeoutSeconds = "180"
	if err := savePreferences(dir, prefs); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPreferences(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != prefs {
		t.Fatalf("saved generation controls did not round trip: got %#v want %#v", loaded, prefs)
	}
}

func TestPreferencesRejectUnsupportedGenerationControls(t *testing.T) {
	prefs := defaultPreferences()
	prefs.ReasoningEffort = "maximum turbo"
	if err := savePreferences(t.TempDir(), prefs); err == nil {
		t.Fatal("unsupported generation option was saved")
	}
}
