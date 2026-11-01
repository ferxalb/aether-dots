package desktopui

import (
	"testing"

	"aether-dots/internal/task"
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
	if !tester.HasText("New Dot") || !tester.HasText("Outcome") || tester.HasText("Capture a task") {
		t.Fatalf("compact capture composer missing or legacy heading remains: %q", tester.Texts())
	}
	for _, label := range []string{"Local access", "Read", "Write", "Choose a source folder", "Choose an output folder", "Execution", "Local extraction"} {
		if !tester.HasText(label) {
			t.Fatalf("capture view missing %q: %q", label, tester.Texts())
		}
	}
	a.settingsOpen = true
	settings := ui.NewTester(a.view, 1120, 760)
	if !settings.HasText("Appearance") || !settings.HasText("Official OpenAI Go SDK · Responses API") {
		t.Fatalf("settings page missing appearance or official OpenAI configuration: %q", settings.Texts())
	}
}

func TestRunRoutineUsesSelectedSpace(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaces := db.Snapshot().Spaces
	if len(spaces) == 0 {
		t.Fatal("store has no initial space")
	}
	target, err := db.CreateSpace("Research")
	if err != nil {
		t.Fatal(err)
	}
	dot, err := db.CreateDot(task.CreateDot{
		SpaceID: spaces[0].ID, Objective: "Prepare a concise brief", Mode: "local",
		SourceRoot: "/tmp/source", OutputDir: "/tmp/output",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Done, "Completed."); err != nil {
		t.Fatal(err)
	}
	if err := db.AddArtifact(dot.ID, task.Artifact{Path: "/tmp/output/brief.md", SHA256: "abc", Bytes: 10}); err != nil {
		t.Fatal(err)
	}
	routine, err := db.PromoteRoutine(dot.ID, "Research brief")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{
		db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local",
		selectedSpaceID: target.ID, selectedRoutineID: routine.ID,
		routineName: routine.Name, routineObjective: routine.Objective,
	}
	a.refresh()
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Run Routine"); err != nil {
		t.Fatal(err)
	}
	dots := db.Snapshot().Dots
	if len(dots) != 2 {
		t.Fatalf("got %d Dots, want 2", len(dots))
	}
	created := dots[1]
	if created.SpaceID != target.ID {
		t.Fatalf("Run Routine created Dot in space %q, want selected space %q", created.SpaceID, target.ID)
	}
	if created.RoutineID != routine.ID {
		t.Fatalf("Run Routine lost Routine link %q, want %q", created.RoutineID, routine.ID)
	}
}

func TestCaptureComposerCreatesDotWithSelectedScopesAndMode(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaces := db.Snapshot().Spaces
	if len(spaces) == 0 {
		t.Fatal("store has no initial space")
	}
	sourceRoot, outputDir := t.TempDir(), t.TempDir()
	a := &app{
		db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local",
		sourceRoot: sourceRoot, outputDir: outputDir,
	}
	a.refresh()
	tester := ui.NewTester(a.view, 760, 560)

	if err := tester.Click("Local extraction"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("OpenAI model"); err != nil {
		t.Fatal(err)
	}
	if a.mode != "model" {
		t.Fatalf("selected mode %q, want model", a.mode)
	}
	if !tester.HasText(a.executionDisclosure()) {
		t.Fatalf("model disclosure missing after changing execution mode: %q", tester.Texts())
	}

	if err := tester.Click("Outcome"); err != nil {
		t.Fatal(err)
	}
	tester.Type("Prepare a concise decision brief.")
	if a.objective != "Prepare a concise decision brief." {
		t.Fatalf("typed outcome %q", a.objective)
	}
	if err := tester.Click("Create Dot"); err != nil {
		t.Fatal(err)
	}
	dots := db.Snapshot().Dots
	if len(dots) != 1 {
		t.Fatalf("got %d Dots, want 1", len(dots))
	}
	created := dots[0]
	if created.Objective != a.objective || created.Mode != "model" {
		t.Fatalf("created Dot did not retain outcome/mode: %+v", created)
	}
	if created.SpaceID != spaces[0].ID || created.SourceRoot != sourceRoot || created.OutputDir != outputDir {
		t.Fatalf("created Dot did not retain selected local scope: %+v", created)
	}
}
