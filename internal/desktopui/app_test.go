package desktopui

import (
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceCreatesDotImmediatelyAndOpensChat(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("A clear space for focused work") {
		t.Fatalf("empty workspace welcome is missing: %q", tester.Texts())
	}
	if tester.HasText("Local access") || tester.HasText("Choose a folder") || tester.HasText("Execution") {
		t.Fatalf("welcome still exposes local folder setup: %q", tester.Texts())
	}
	if err := tester.Click("New Dot"); err != nil {
		t.Fatal(err)
	}
	snapshot := a.db.Snapshot()
	if len(snapshot.Conversations) != 1 {
		t.Fatalf("New Dot created %d profiles, want one", len(snapshot.Conversations))
	}
	created := snapshot.Conversations[0]
	if created.Name != "New Dot" || created.Purpose != "" || created.Icon == "" || created.Color == "" {
		t.Fatalf("new Dot should be immediately created with editable defaults: %#v", created)
	}
	if a.selectedConversationID != created.ID || !a.hasConversationChat() || !tester.HasText("Start with a prompt") {
		t.Fatalf("new Dot did not open its chat: selected=%q route=%v texts=%q", a.selectedConversationID, a.hasConversationChat(), tester.Texts())
	}
	for _, text := range tester.Texts() {
		if text == "Read" || text == "Write" || text == "Choose a folder" {
			t.Fatalf("new Dot chat exposes local folder access: %q", tester.Texts())
		}
	}
}

func TestDotProfileCanBeCustomizedFromChat(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation := createTestConversation(t, a, a.selectedSpaceID, "New Dot", "")
	a.selectConversation(conversation.ID)
	tester := ui.NewTester(a.view, 960, 720)
	if err := tester.Click("Customize"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Customize your Dot") || !tester.HasText("Objective") || !tester.HasText("Working instructions") {
		t.Fatalf("profile editor is missing core fields: %q", tester.Texts())
	}
	if err := tester.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("Decision partner")
	if err := tester.Click("Objective"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("Prepare concise decision briefs")
	if err := tester.Click("Working instructions"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("Use plain language. State uncertainty.")
	if a.profileName != "Decision partner" || a.profilePurpose != "Prepare concise decision briefs" || a.profileInstructions != "Use plain language. State uncertainty." {
		t.Fatalf("profile inputs did not retain edits: name=%q objective=%q instructions=%q", a.profileName, a.profilePurpose, a.profileInstructions)
	}
	clickUIControl(t, tester, "Save profile", 960, 720)
	updated, ok := findConversation(a.db.Snapshot(), conversation.ID)
	if !ok || updated.Name != "Decision partner" || updated.Purpose != "Prepare concise decision briefs" || updated.Instructions != "Use plain language. State uncertainty." {
		t.Fatalf("profile did not persist: %#v", updated)
	}
}

func TestSettingsCanReturnToSelectedDotChat(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation := createTestConversation(t, a, a.selectedSpaceID, "Persistent selection", "Keep this Dot selected.")
	a.selectConversation(conversation.ID)
	a.credentialChecked = true
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || a.selectedConversationID != conversation.ID {
		t.Fatalf("opening Settings lost selection: settings=%v conversation=%q", a.settingsOpen, a.selectedConversationID)
	}
	settings := ui.NewTester(a.view, 1120, 760)
	if !settings.HasText("Appearance") || !settings.HasText("Choose how AETHER looks and how your Dots work.") ||
		settings.HasText("Chat only · no file access") {
		t.Fatalf("settings overview is missing user-facing appearance settings or retains the old sidebar slogan: %q", settings.Texts())
	}
	for _, section := range []struct {
		label, content string
	}{
		{"AI & responses", "Choose a provider for replies and scheduled runs."},
		{"Skills", "Save reusable instructions and choose when a Dot should use them."},
		{"Routines", "Have a Dot repeat a prompt on a schedule."},
	} {
		if err := settings.Click(section.label); err != nil {
			t.Fatal(err)
		}
		if !settings.HasText(section.content) {
			t.Fatalf("settings section %q is missing its content: %q", section.label, settings.Texts())
		}
	}
	if err := settings.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen || !a.hasConversationChat() || a.selectedConversationID != conversation.ID {
		t.Fatalf("closing Settings did not return to chat: settings=%v conversation=%q", a.settingsOpen, a.selectedConversationID)
	}
}

func TestSettingsCanSaveSharedSkillAndRoutine(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation := createTestConversation(t, a, a.selectedSpaceID, "Automation Dot", "Use the latest chat context.")
	a.selectConversation(conversation.ID)
	a.settingsOpen = true
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Skills"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("New skill"); err != nil {
		t.Fatal(err)
	}
	a.skillDraft = task.Skill{
		Name: "Weekly review", Description: "Review the week's work.",
		WhenToUse: "When asked for a weekly review.", Inputs: "Use facts in the chat.",
		Steps: "Group progress and risks.", Validation: "Check every statement against the supplied facts.",
		Output: "Return a short summary.", Approval: "Do not contact anyone or take external actions.",
	}
	clickUIControl(t, tester, "Save skill", 1120, 760)
	if len(a.db.Snapshot().Skills) != 1 {
		t.Fatal("skill form did not save the reusable playbook")
	}
	if err := tester.Click("Routines"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("New routine"); err != nil {
		t.Fatal(err)
	}
	a.routineNameDraft = "Daily review"
	a.routinePromptDraft = "Summarize the current context and report missing information."
	a.routineDotDraft = conversation.ID
	a.routineEveryDraft = "30m"
	clickUIControl(t, tester, "Save routine", 1120, 760)
	snapshot := a.db.Snapshot()
	if len(snapshot.Routines) != 1 || snapshot.Routines[0].ConversationID != conversation.ID || snapshot.Routines[0].IntervalSeconds != 1800 {
		t.Fatalf("routine form did not save the interval schedule: %#v", snapshot.Routines)
	}
}

func newMessengerTestApp(t *testing.T) *app {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, dataDir: t.TempDir(), prefs: defaultPreferences()}
	a.refresh()
	return a
}

func createTestConversation(t *testing.T, a *app, spaceID, name, purpose string) task.Conversation {
	t.Helper()
	conversation, err := a.db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: name, Purpose: purpose,
	})
	if err != nil {
		t.Fatal(err)
	}
	return conversation
}

func clickUIControl(t *testing.T, tester *ui.Tester, label string, width, height int) {
	t.Helper()
	// Long forms (the Dot profile editor is ~2,100px tall) need many wheel
	// steps at a small viewport, so allow enough of them to cover the page.
	for attempt := 0; attempt < 60; attempt++ {
		bounds, ok := tester.Find(label)
		if !ok {
			t.Fatalf("control %q is not present: %q", label, tester.Texts())
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
		tester.Scroll(float32(width)-40, float32(height)-36, 0, dy)
		tester.Frame()
	}
	t.Fatalf("control %q did not scroll into the %dx%d viewport", label, width, height)
}
