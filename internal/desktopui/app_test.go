package desktopui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"aether-dots/internal/store"
)

func TestWorkspaceViewShowsCaptureScopeAndModelStatus(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, mode: "local"}
	a.refresh()
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Capture a task") {
		t.Fatalf("capture view missing: %q", tester.Texts())
	}
	if !tester.HasText("Choose source…") || !tester.HasText("Choose destination…") {
		t.Fatalf("explicit source/output controls missing: %q", tester.Texts())
	}
	if !tester.HasText("OpenAI-compatible model") {
		t.Fatalf("model configuration status missing: %q", tester.Texts())
	}
}
