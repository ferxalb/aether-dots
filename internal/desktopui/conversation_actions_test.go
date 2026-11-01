package desktopui

import (
	"testing"

	"aether-dots/internal/task"
)

func TestNewDotActionCreatesImmediatelyWithoutASetupForm(t *testing.T) {
	a := newMessengerTestApp(t)
	spaceID := a.selectedSpaceID
	a.startCreator()
	snapshot := a.db.Snapshot()
	if len(snapshot.Conversations) != 1 {
		t.Fatalf("new Dot action created %d profiles, want one", len(snapshot.Conversations))
	}
	created := snapshot.Conversations[0]
	if created.SpaceID != spaceID || created.Name != "New Dot" || created.Purpose != "" {
		t.Fatalf("unexpected new Dot defaults: %#v", created)
	}
	if a.selectedConversationID != created.ID || !a.hasConversationChat() {
		t.Fatalf("new Dot was not opened in chat: selected=%q route=%v", a.selectedConversationID, a.hasConversationChat())
	}
}

func TestProfileSaveUpdatesDurableDotSettings(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation, err := a.db.CreateConversation(task.CreateConversation{SpaceID: a.selectedSpaceID})
	if err != nil {
		t.Fatal(err)
	}
	a.refresh()
	a.selectConversation(conversation.ID)
	a.openConversationProfile()
	a.profileName = "Writing partner"
	a.profileIcon = "✧"
	a.profileColor = "#C33E70"
	a.profilePurpose = "Help me write clear product updates."
	a.profileInstructions = "Be concise, warm, and specific."
	a.saveConversationProfile()
	updated, ok := findConversation(a.db.Snapshot(), conversation.ID)
	if !ok || updated.Name != a.profileName || updated.Icon != "✧" || updated.Color != "#C33E70" || updated.Purpose != a.profilePurpose || updated.Instructions != a.profileInstructions {
		t.Fatalf("saved Dot profile does not match edits: %#v", updated)
	}
	if a.conversationDetailsOpen {
		t.Fatal("successful save did not return to chat")
	}
}

func TestProfileCanMoveDotIntoNewSpace(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation, err := a.db.CreateConversation(task.CreateConversation{SpaceID: a.selectedSpaceID})
	if err != nil {
		t.Fatal(err)
	}
	a.refresh()
	a.selectConversation(conversation.ID)
	a.openConversationProfile()
	a.openSpaceCreate()
	a.spaceDraft = "Research"
	a.saveSpaceDraft()
	spaceID := a.profileSpaceID
	if !containsSpace(a.db.Snapshot(), spaceID) {
		t.Fatalf("new Space was not persisted: %q", spaceID)
	}
	a.saveConversationProfile()
	updated, ok := findConversation(a.db.Snapshot(), conversation.ID)
	if !ok || updated.SpaceID != spaceID {
		t.Fatalf("Dot did not move to the selected Space: %#v", updated)
	}
}
