package desktopui

import (
	"testing"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"

	"aether-dots/internal/store"
)

func TestWorkspaceWelcomeOpensSeparateCreatorAndSettings(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Create a Dot to begin") || tester.HasText("Purpose") {
		t.Fatalf("empty workspace should show the welcome route, not the creator form: %q", tester.Texts())
	}
	if err := tester.Click("Create a Dot"); err != nil {
		t.Fatal(err)
	}
	if !a.creatorOpen || !tester.HasText("Dot name") || !tester.HasText("Purpose") {
		t.Fatalf("Create a Dot did not open the separate creator route: creator=%v texts=%q", a.creatorOpen, tester.Texts())
	}
	for _, label := range []string{"Local access", "Read", "Write", "Choose a source folder", "Choose an output folder", "Execution"} {
		if !tester.HasText(label) {
			t.Fatalf("creator missing %q: %q", label, tester.Texts())
		}
	}

	conversation := createTestConversation(t, a, a.selectedSpaceID, "Persistent selection", "Keep this conversation selected.")
	a.selectConversation(conversation.ID)
	// Keep this UI test isolated from the host's system credential store.
	a.credentialChecked = true
	tester = ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || a.selectedConversationID != conversation.ID {
		t.Fatalf("opening Settings lost the selected conversation: settings=%v conversation=%q", a.settingsOpen, a.selectedConversationID)
	}
	settings := ui.NewTester(a.view, 1120, 760)
	if !settings.HasText("Appearance") || !settings.HasText("Official OpenAI Go SDK · Responses API") {
		t.Fatalf("settings page missing appearance or official OpenAI configuration: %q", settings.Texts())
	}
	if err := settings.Click(conversation.Name); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen || a.selectedConversationID != conversation.ID || !a.hasConversationChat() {
		t.Fatalf("returning from Settings did not reopen the selected chat: settings=%v conversation=%q route=%v", a.settingsOpen, a.selectedConversationID, a.hasConversationChat())
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

func TestCreatorCreatesNamedConversationWithoutStartingTask(t *testing.T) {
	a := newMessengerTestApp(t)
	spaceID := a.selectedSpaceID
	sourceRoot, outputDir := t.TempDir(), t.TempDir()
	a.startCreator()
	a.sourceRoot, a.outputDir = sourceRoot, outputDir
	tester := ui.NewTester(a.view, 760, 560)

	if err := tester.Click("Dot name"); err != nil {
		t.Fatal(err)
	}
	tester.Type("Design review")
	if a.dotName != "Design review" {
		t.Fatalf("typed Dot name %q", a.dotName)
	}
	if err := tester.Click("Purpose"); err != nil {
		t.Fatal(err)
	}
	tester.Type("Prepare a concise decision brief.")
	if a.objective != "Prepare a concise decision brief." {
		t.Fatalf("typed Dot purpose %q", a.objective)
	}
	clickCreatorControl(t, tester, "Local extraction", 760, 560)
	if err := tester.Click("OpenAI model"); err != nil {
		t.Fatal(err)
	}
	if a.mode != "model" {
		t.Fatalf("selected mode %q, want model", a.mode)
	}
	if !tester.HasText(creatorExecutionDisclosureFor(a.mode)) {
		t.Fatalf("model disclosure missing after changing execution mode: %q", tester.Texts())
	}

	clickCreatorControl(t, tester, "Create Dot", 760, 560)
	db := a.db.Snapshot()
	if len(db.Conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(db.Conversations))
	}
	if len(db.Dots) != 0 {
		t.Fatalf("creating a Dot unexpectedly started %d task(s)", len(db.Dots))
	}
	created := db.Conversations[0]
	if created.Name != "Design review" || created.Purpose != "Prepare a concise decision brief." || created.Mode != "model" {
		t.Fatalf("created conversation did not retain name, purpose, and mode: %+v", created)
	}
	if created.SpaceID != spaceID || created.SourceRoot != sourceRoot || created.OutputDir != outputDir {
		t.Fatalf("created conversation did not retain selected space and local scopes: %+v", created)
	}
	if a.creatorOpen || a.selectedConversationID != created.ID || !a.hasConversationChat() {
		t.Fatalf("created Dot did not open its empty chat: creator=%v selected=%q route=%v", a.creatorOpen, a.selectedConversationID, a.hasConversationChat())
	}
}

func clickCreatorControl(t *testing.T, tester *ui.Tester, label string, width, height int) {
	t.Helper()
	for attempt := 0; attempt < 8; attempt++ {
		bounds, ok := tester.Find(label)
		if !ok {
			t.Fatalf("creator control %q is not present; visible text: %q", label, tester.Texts())
		}
		if bounds.Y >= 0 && bounds.Y+bounds.H <= float32(height) {
			if err := tester.Click(label); err != nil {
				t.Fatal(err)
			}
			return
		}
		dy := float32(120)
		if bounds.Y+bounds.H <= 0 {
			dy = -120
		}
		tester.Scroll(float32(width)-48, float32(height)-24, 0, dy)
		tester.Frame()
	}
	t.Fatalf("creator control %q did not scroll into the %dx%d viewport", label, width, height)
}
