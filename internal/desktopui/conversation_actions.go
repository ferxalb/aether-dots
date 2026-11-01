package desktopui

import (
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) startCreator() {
	a.rememberCurrentConversationState()
	if !containsSpace(a.snapshot, a.selectedSpaceID) && len(a.snapshot.Spaces) > 0 {
		a.selectedSpaceID = a.snapshot.Spaces[0].ID
	}
	spaceID := a.selectedSpaceID
	if spaceID == "" && len(a.snapshot.Spaces) > 0 {
		spaceID = a.snapshot.Spaces[0].ID
	}
	conversation, err := a.db.CreateConversation(task.CreateConversation{SpaceID: spaceID})
	if err != nil {
		a.message = err.Error()
		return
	}
	a.settingsOpen = false
	a.conversationDetailsOpen = false
	a.selectedDotID = ""
	a.message = "Dot created. Start with a prompt, then personalize its profile whenever you like."
	a.refresh()
	a.selectConversation(conversation.ID)
	a.message = "Dot created. Start with a prompt, then personalize its profile whenever you like."
}

func (a *app) openConversationProfile() {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		return
	}
	a.profileName = conversation.Name
	a.profilePurpose = conversation.Purpose
	a.profileInstructions = conversation.Instructions
	a.profileIcon = conversation.Icon
	a.profileColor = conversation.Color
	a.profileSpaceID = conversation.SpaceID
	a.spaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
	a.conversationError = ""
	a.conversationDetailsOpen = true
	a.settingsOpen = false
}

func (a *app) saveConversationProfile() {
	err := a.db.UpdateConversationProfile(
		a.selectedConversationID,
		a.profileSpaceID,
		a.profileName,
		a.profileIcon,
		a.profileColor,
		a.profilePurpose,
		a.profileInstructions,
	)
	if err != nil {
		a.conversationError = err.Error()
		return
	}
	a.selectedSpaceID = a.profileSpaceID
	a.conversationError = ""
	a.conversationDetailsOpen = false
	a.message = "Dot profile saved. New replies will follow these instructions."
	a.refresh()
}

func (a *app) sendConversationMessage() {
	if a.cancelRun != nil {
		a.conversationError = "Wait for the current reply to finish or stop it before sending another message."
		return
	}
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.conversationError = "Choose a Dot before sending a message."
		return
	}
	dot, _, err := a.db.CreateConversationRun(conversation.ID, a.messageDraft)
	if err != nil {
		a.conversationError = err.Error()
		return
	}
	a.clearConversationDraft(conversation.ID)
	a.messageDraft = ""
	a.conversationError = ""
	a.message = "Message sent to " + conversation.Name + "."
	a.refresh()
	a.chatScroll.ScrollTo(0, 1e30, ui.Smooth)
	a.startRun(dot.ID)
}

func (a *app) stopConversationRun() {
	if a.cancelRun == nil {
		return
	}
	a.cancelRun()
	a.message = "Stopping the current reply…"
}

func (a *app) retryConversationMessage(dotID string) {
	if a.cancelRun != nil {
		return
	}
	if err := a.db.SetStatus(dotID, task.Ready, "Retry requested by the user."); err != nil {
		a.conversationError = err.Error()
		return
	}
	a.refresh()
	a.startRun(dotID)
}
