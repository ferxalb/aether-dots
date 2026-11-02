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
		a.adt.Error("Couldn't create a Dot", err.Error())
		return
	}
	a.settingsOpen = false
	a.conversationDetailsOpen = false
	a.selectedDotID = ""
	a.refresh()
	a.selectConversation(conversation.ID)
	a.adt.Success("Dot created", "Start with a prompt, then personalize its profile whenever you like.")
}

func (a *app) openConversationProfile() {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		return
	}
	a.profileName = conversation.Name
	a.profilePurpose = conversation.Purpose
	a.profileInstructions = conversation.Instructions
	a.profileDelegateIDs = append([]string(nil), conversation.DelegatedDotIDs...)
	a.profileGitHubRepositoryIDs = append([]int64(nil), conversation.GitHubRepositoryIDs...)
	a.profileGitHubEnabledDraft = len(conversation.GitHubRepositoryIDs) > 0
	a.profileGitHubDisclosureAccepted = conversation.GitHubModelDisclosureAccepted
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
	githubRepositoryIDs := append([]int64(nil), a.profileGitHubRepositoryIDs...)
	current, _ := findConversation(a.snapshot, a.selectedConversationID)
	if a.profileSpaceID != current.SpaceID || !a.profileGitHubEnabledDraft || !a.profileGitHubDisclosureAccepted {
		githubRepositoryIDs = nil
	}
	err := a.db.UpdateConversationProfileAndGitHub(
		a.selectedConversationID,
		a.profileSpaceID,
		a.profileName,
		a.profileIcon,
		a.profileColor,
		a.profilePurpose,
		a.profileInstructions,
		validProfileDelegateIDs(a.snapshot, a.selectedConversationID, a.profileSpaceID, a.profileDelegateIDs),
		githubRepositoryIDs,
		a.profileGitHubDisclosureAccepted,
	)
	if err != nil {
		a.conversationError = err.Error()
		a.adt.Error("Couldn't save the Dot profile", err.Error())
		return
	}
	a.selectedSpaceID = a.profileSpaceID
	a.conversationError = ""
	a.conversationDetailsOpen = false
	a.adt.Success("Dot profile saved", "New replies will follow these instructions.")
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
	a.refresh()
	a.chatScroll.ScrollTo(0, 1e30, ui.Smooth)
	a.startRun(dot.ID)
}

func (a *app) stopConversationRun() {
	if a.cancelRun == nil {
		return
	}
	a.cancelRun()
	a.adt.Info("Stopping the reply", "Your Dot will stop as soon as it can.")
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
