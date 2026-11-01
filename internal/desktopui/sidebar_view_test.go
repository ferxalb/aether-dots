package desktopui

import (
	"math"
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func newMessengerTestApp(t *testing.T) *app {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local"}
	a.refresh()
	return a
}

func createTestConversation(t *testing.T, a *app, spaceID, name, purpose string) task.Conversation {
	t.Helper()
	sourceRoot, outputDir := t.TempDir(), t.TempDir()
	conversation, err := a.db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: name, Purpose: purpose, Mode: "local",
		SourceRoot: sourceRoot, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

func TestSidebarSearchFindsConversationByLastMessage(t *testing.T) {
	a := newMessengerTestApp(t)
	spaceID := a.snapshot.Spaces[0].ID
	sales := createTestConversation(t, a, spaceID, "Sales Outbound", "Review the selected accounts.")
	if _, _, err := a.db.CreateConversationRun(sales.ID, "Book the venue and send the plan."); err != nil {
		t.Fatal(err)
	}
	createTestConversation(t, a, spaceID, "Inbox Manager", "Triage the incoming messages.")
	a.refresh()
	// Keep the main pane on the welcome route so an unfiltered row cannot be
	// mistaken for the selected conversation title in the chat header.
	a.selectedConversationID = ""
	tester := ui.NewTester(a.view, 960, 720)

	if err := tester.Click("Search Dots"); err != nil {
		t.Fatal(err)
	}
	tester.Type("venue")
	if a.searchQuery != "venue" {
		t.Fatalf("search query = %q, want venue", a.searchQuery)
	}
	if !tester.HasText("Sales Outbound") || tester.HasText("Inbox Manager") {
		t.Fatalf("search did not filter by the last conversation message: %q", tester.Texts())
	}
}

func TestSidebarSelectsConversationAndPreservesItsSpace(t *testing.T) {
	a := newMessengerTestApp(t)
	space, err := a.db.CreateSpace("Research")
	if err != nil {
		t.Fatal(err)
	}
	conversation := createTestConversation(t, a, space.ID, "Research Plan", "Prepare a research brief.")
	a.refresh()
	if a.selectedConversationID != conversation.ID || a.chatScroll.Y != math.MaxFloat32 {
		t.Fatalf("startup navigation = conversation %q, scroll %v; want selected conversation at transcript end", a.selectedConversationID, a.chatScroll.Y)
	}
	tester := ui.NewTester(a.view, 960, 720)

	if err := tester.Click(conversation.Name); err != nil {
		t.Fatal(err)
	}
	if a.selectedConversationID != conversation.ID || a.selectedSpaceID != space.ID {
		t.Fatalf("selection = conversation %q, space %q; want %q, %q", a.selectedConversationID, a.selectedSpaceID, conversation.ID, space.ID)
	}
	if a.creatorOpen || a.settingsOpen || a.selectedDotID != "" {
		t.Fatalf("conversation selection left another route active: creator=%v settings=%v legacyDot=%q", a.creatorOpen, a.settingsOpen, a.selectedDotID)
	}
}

func TestConversationNavigationRestoresDraftPerDot(t *testing.T) {
	a := newMessengerTestApp(t)
	spaceID := a.selectedSpaceID
	first := createTestConversation(t, a, spaceID, "First Dot", "First purpose")
	second := createTestConversation(t, a, spaceID, "Second Dot", "Second purpose")
	a.refresh()

	a.selectConversation(first.ID)
	a.messageDraft = "Continue the first task"
	a.selectConversation(second.ID)
	if a.messageDraft != "" {
		t.Fatalf("switching to another Dot retained the previous draft: %q", a.messageDraft)
	}
	a.messageDraft = "Ask the second Dot"
	a.selectConversation(first.ID)
	if a.messageDraft != "Continue the first task" {
		t.Fatalf("first Dot draft = %q, want its saved draft", a.messageDraft)
	}
	a.selectConversation(second.ID)
	if a.messageDraft != "Ask the second Dot" {
		t.Fatalf("second Dot draft = %q, want its saved draft", a.messageDraft)
	}
}

func TestSidebarPlusOpensDotCreator(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 960, 720)

	if !tester.HasText("Create a Dot") {
		t.Fatalf("sidebar creation button missing: %q", tester.Texts())
	}
	if err := tester.Click("Create a Dot"); err != nil {
		t.Fatal(err)
	}
	if !a.creatorOpen || a.selectedConversationID != "" {
		t.Fatalf("plus did not open the separate creator route: creator=%v conversation=%q", a.creatorOpen, a.selectedConversationID)
	}
}

func TestLegacyDotRemainsReachableUnderOlderDots(t *testing.T) {
	a := newMessengerTestApp(t)
	dot, err := a.db.CreateDot(task.CreateDot{
		SpaceID: a.snapshot.Spaces[0].ID, Name: "Previous summary", Objective: "Review old notes",
		Mode: "local", SourceRoot: "/tmp/source", OutputDir: "/tmp/output",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.refresh()
	tester := ui.NewTester(a.view, 960, 720)

	if !tester.HasText("Older Dots") || !tester.HasText(dot.Name) {
		t.Fatalf("legacy Dot missing from the sidebar: %q", tester.Texts())
	}
	if err := tester.Click(dot.Name); err != nil {
		t.Fatal(err)
	}
	if a.selectedDotID != dot.ID || a.selectedConversationID != "" {
		t.Fatalf("legacy Dot navigation = Dot %q, conversation %q", a.selectedDotID, a.selectedConversationID)
	}
}

func TestSidebarRoutinesLinkPreservesLegacyRoutineAccess(t *testing.T) {
	a := newMessengerTestApp(t)
	sourceRoot, outputDir := t.TempDir(), t.TempDir()
	dot, err := a.db.CreateDot(task.CreateDot{
		SpaceID: a.selectedSpaceID, Name: "Finished brief", Objective: "Prepare a short brief",
		Mode: "local", SourceRoot: sourceRoot, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.SetStatus(dot.ID, task.Done, "Completed."); err != nil {
		t.Fatal(err)
	}
	if err := a.db.AddArtifact(dot.ID, task.Artifact{Path: outputDir + "/brief.md", SHA256: "abc", Bytes: 10}); err != nil {
		t.Fatal(err)
	}
	routine, err := a.db.PromoteRoutine(dot.ID, "Brief template")
	if err != nil {
		t.Fatal(err)
	}
	a.selectedDotID = dot.ID
	a.refresh()
	tester := ui.NewTester(a.view, 960, 720)

	if err := tester.Click("Routines"); err != nil {
		t.Fatal(err)
	}
	if !a.routinesOpen || !tester.HasText("Saved Routines") || !tester.HasText(routine.Name) {
		t.Fatalf("sidebar Routines link did not show the saved routine: route=%v texts=%q", a.routinesOpen, tester.Texts())
	}
	if err := tester.Click(routine.Name); err != nil {
		t.Fatal(err)
	}
	if a.selectedRoutineID != routine.ID || !tester.HasText("Run Routine") {
		t.Fatalf("saved Routine did not open its editor: selected=%q texts=%q", a.selectedRoutineID, tester.Texts())
	}
	if err := tester.Click("Back to routines"); err != nil {
		t.Fatal(err)
	}
	if a.selectedRoutineID != "" || !a.routinesOpen {
		t.Fatalf("Routine editor did not return to its list: selected=%q list=%v", a.selectedRoutineID, a.routinesOpen)
	}
	if err := tester.Click("Back to Dot"); err != nil {
		t.Fatal(err)
	}
	if a.routinesOpen || a.selectedDotID != dot.ID {
		t.Fatalf("Routine list did not restore the legacy Dot route: list=%v dot=%q", a.routinesOpen, a.selectedDotID)
	}
}
