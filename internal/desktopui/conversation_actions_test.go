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

func TestComputerUseAccessChangesControlBackgroundWarmup(t *testing.T) {
	tests := []struct {
		name               string
		previousScopes     []string
		nextScopes         []string
		previousDisclosure bool
		nextDisclosure     bool
		wantAdded          bool
		wantReduced        bool
	}{
		{name: "unchanged grant", previousScopes: []string{"notes"}, nextScopes: []string{"notes"}, previousDisclosure: true, nextDisclosure: true},
		{name: "new scope", nextScopes: []string{"notes"}, nextDisclosure: true, wantAdded: true},
		{name: "new disclosure", previousScopes: []string{"notes"}, nextScopes: []string{"notes"}, nextDisclosure: true, wantAdded: true},
		{name: "scope removed", previousScopes: []string{"notes"}, previousDisclosure: true, nextDisclosure: true, wantReduced: true},
		{name: "disclosure revoked", previousScopes: []string{"notes"}, nextScopes: []string{"notes"}, previousDisclosure: true, wantReduced: true},
		{name: "scope replaced", previousScopes: []string{"notes"}, nextScopes: []string{"music"}, previousDisclosure: true, nextDisclosure: true, wantAdded: true, wantReduced: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added := computerUseAccessAdded(tt.previousScopes, tt.nextScopes, tt.previousDisclosure, tt.nextDisclosure)
			reduced := computerUseAccessReduced(tt.previousScopes, tt.nextScopes, tt.previousDisclosure, tt.nextDisclosure)
			if added != tt.wantAdded || reduced != tt.wantReduced {
				t.Fatalf("access change = (added %v, reduced %v), want (added %v, reduced %v)", added, reduced, tt.wantAdded, tt.wantReduced)
			}
		})
	}
}

func TestComputerUseWarmupRequiresAnInstalledDriverAndDisclosedGrant(t *testing.T) {
	valid := func(added, reduced bool, scopes []string, disclosure, installed bool) bool {
		return computerUseShouldPrepare(added, reduced, scopes, disclosure, installed)
	}
	if !valid(true, false, []string{"notes"}, true, true) {
		t.Fatal("new, disclosed grant with an installed driver should warm in background")
	}
	for _, tt := range []struct {
		name       string
		added      bool
		reduced    bool
		scopes     []string
		disclosure bool
		installed  bool
	}{
		{name: "no new grant", scopes: []string{"notes"}, disclosure: true, installed: true},
		{name: "undisclosed grant", added: true, scopes: []string{"notes"}, installed: true},
		{name: "no scopes", added: true, disclosure: true, installed: true},
		{name: "driver missing", added: true, scopes: []string{"notes"}, disclosure: true},
		{name: "access reduced", added: true, reduced: true, scopes: []string{"notes"}, disclosure: true, installed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if valid(tt.added, tt.reduced, tt.scopes, tt.disclosure, tt.installed) {
				t.Fatal("background warm-up ran without an eligible grant")
			}
		})
	}
}
