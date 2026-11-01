package desktopui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"aether-dots/internal/store"
)

func TestWorkspaceViewKeepsModelConfigurationInSettings(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local"}
	a.refresh()
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Capture a task") {
		t.Fatalf("capture view missing: %q", tester.Texts())
	}
	if !tester.HasText("Choose source…") || !tester.HasText("Choose destination…") {
		t.Fatalf("explicit source/output controls missing: %q", tester.Texts())
	}
	a.settingsOpen = true
	settings := ui.NewTester(a.view, 1120, 760)
	if !settings.HasText("Appearance") || !settings.HasText("Official OpenAI Go SDK · Responses API") {
		t.Fatalf("settings page missing appearance or official OpenAI configuration: %q", settings.Texts())
	}
}
