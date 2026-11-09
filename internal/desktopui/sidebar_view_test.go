package desktopui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarSearchFindsConversationByLastMessage(t *testing.T) {
	a := newMessengerTestApp(t)
	spaceID := a.snapshot.Spaces[0].ID
	sales := createTestConversation(t, a, spaceID, "Sales Outbound", "Review the accounts.")
	if _, _, err := a.db.CreateConversationRun(sales.ID, "Prepare the event plan."); err != nil {
		t.Fatal(err)
	}
	createTestConversation(t, a, spaceID, "Inbox Manager", "Triage incoming messages.")
	a.refresh()
	a.selectedConversationID = ""
	tester := ui.NewTester(a.view, 960, 720)
	if err := tester.Click("Search Dots"); err != nil {
		t.Fatal(err)
	}
	tester.Type("event plan")
	if a.searchQuery != "event plan" {
		t.Fatalf("search query = %q", a.searchQuery)
	}
	if !tester.HasText("Sales Outbound") || tester.HasText("Inbox Manager") {
		t.Fatalf("search did not find a recent message: %q", tester.Texts())
	}
}

func TestSidebarSelectsConversationAndPreservesSpace(t *testing.T) {
	a := newMessengerTestApp(t)
	space, err := a.db.CreateSpace("Research")
	if err != nil {
		t.Fatal(err)
	}
	conversation := createTestConversation(t, a, space.ID, "Research Plan", "Prepare a research brief.")
	a.refresh()
	if a.selectedConversationID != conversation.ID || !a.chatMessages.FollowEnd {
		t.Fatalf("startup navigation = conversation %q, follows end %v", a.selectedConversationID, a.chatMessages.FollowEnd)
	}
	tester := ui.NewTester(a.view, 960, 720)
	if err := tester.Click(conversation.Name); err != nil {
		t.Fatal(err)
	}
	if a.selectedConversationID != conversation.ID || a.selectedSpaceID != space.ID {
		t.Fatalf("selection = conversation %q, space %q", a.selectedConversationID, a.selectedSpaceID)
	}
	if a.settingsOpen || a.selectedDotID != "" || a.conversationDetailsOpen {
		t.Fatalf("conversation selection left another route active")
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
		t.Fatalf("switching Dots retained the previous draft: %q", a.messageDraft)
	}
	a.messageDraft = "Ask the second Dot"
	a.selectConversation(first.ID)
	if a.messageDraft != "Continue the first task" {
		t.Fatalf("first Dot draft = %q", a.messageDraft)
	}
	a.selectConversation(second.ID)
	if a.messageDraft != "Ask the second Dot" {
		t.Fatalf("second Dot draft = %q", a.messageDraft)
	}
}

func TestSidebarNewDotButtonCreatesAndSelectsChat(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 960, 720)
	if !tester.HasText("New Dot") {
		t.Fatalf("New Dot button missing: %q", tester.Texts())
	}
	if err := tester.Click("New Dot"); err != nil {
		t.Fatal(err)
	}
	snapshot := a.db.Snapshot()
	if len(snapshot.Conversations) != 1 || a.selectedConversationID != snapshot.Conversations[0].ID || !a.hasConversationChat() {
		t.Fatalf("New Dot did not create and open a chat: conversations=%d selection=%q", len(snapshot.Conversations), a.selectedConversationID)
	}
}

func TestCommandNCreatesDotDirectly(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 960, 720)
	tester.Key(ui.Cmd, ui.KeyN)
	snapshot := a.db.Snapshot()
	if len(snapshot.Conversations) != 1 || a.selectedConversationID != snapshot.Conversations[0].ID || !a.hasConversationChat() {
		t.Fatalf("Command-N did not create and open a Dot: conversations=%d selection=%q", len(snapshot.Conversations), a.selectedConversationID)
	}
}

func TestProfileLayoutKeepsChatAtWideWidthsAndPreservesDraft(t *testing.T) {
	for _, tc := range []struct {
		name      string
		width     int
		inspector bool
	}{
		{"wide", 960, true},
		{"minimum", 760, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newMessengerTestApp(t)
			dot := createTestConversation(t, a, a.selectedSpaceID, "Research", "Review new results.")
			a.selectConversation(dot.ID)
			a.messageDraft = "An unsent message"
			tester := ui.NewTester(a.view, tc.width, 560)
			if err := tester.Click("Customize"); err != nil {
				t.Fatal(err)
			}
			_, chatVisible := tester.Find("Message Research")
			if chatVisible != tc.inspector || !tester.HasText("Save profile") {
				t.Fatalf("chat visible = %v, expected %v, profile save visible = %v", chatVisible, tc.inspector, tester.HasText("Save profile"))
			}
			if err := tester.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			if !a.hasConversationChat() || a.messageDraft != "An unsent message" {
				t.Fatalf("closing profile left chat = %v, draft = %q", a.hasConversationChat(), a.messageDraft)
			}
		})
	}
}

func TestCommandKFocusesDotSearch(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 960, 720)
	tester.Key(ui.Cmd, ui.KeyK)
	tester.Type("Research")
	if a.searchQuery != "Research" {
		t.Fatalf("search query = %q", a.searchQuery)
	}
}
