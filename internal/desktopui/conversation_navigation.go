package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) rememberCurrentConversationState() {
	if a.selectedConversationID == "" {
		return
	}
	if a.messageDrafts == nil {
		a.messageDrafts = make(map[string]string)
	}
	a.messageDrafts[a.selectedConversationID] = a.messageDraft
}

func (a *app) selectConversation(id string) {
	if id != a.selectedConversationID {
		a.rememberCurrentConversationState()
		a.selectedConversationID = id
		a.chatMessages = ui.ListState{FollowEnd: true}
		a.messageDraft = a.messageDrafts[id]
	}
	if conversation, ok := findConversation(a.snapshot, id); ok {
		a.selectedSpaceID = conversation.SpaceID
	}
	a.selectedDotID = ""
	a.conversationDetailsOpen = false
	a.settingsOpen = false
	a.conversationError = ""
	a.refresh()
}

func (a *app) clearConversationDraft(id string) {
	if a.messageDrafts != nil {
		delete(a.messageDrafts, id)
	}
	if id == a.selectedConversationID {
		a.messageDraft = ""
	}
}
