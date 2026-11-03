package desktopui

import "aether-dots/internal/task"

func (a *app) startCreator() {
	a.rememberCurrentConversationState()
	a.activityOpen = false
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
	a.profileContextSummary = conversation.ContextSummary
	a.profileDelegateIDs = append([]string(nil), conversation.DelegatedDotIDs...)
	a.profileGitHubRepositoryIDs = append([]int64(nil), conversation.GitHubRepositoryIDs...)
	a.profileGitHubEnabledDraft = len(conversation.GitHubRepositoryIDs) > 0
	a.profileGitHubDisclosureAccepted = conversation.GitHubModelDisclosureAccepted
	a.profileComputerUseScopes = append([]string(nil), conversation.ComputerUseScopes...)
	a.profileComputerUseDisclosureAccepted = conversation.ComputerUseModelDisclosureAccepted
	a.profileIcon = conversation.Icon
	a.profileColor = conversation.Color
	a.profileSpaceID = conversation.SpaceID
	a.spaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
	a.spaceContextDraft = ""
	a.spaceContextTargetID = ""
	a.conversationError = ""
	a.conversationDetailsOpen = true
	a.settingsOpen = false
	a.activityOpen = false
}

func (a *app) saveConversationProfile() {
	if len(a.profileContextSummary) > task.MaxConversationSummaryBytes {
		a.conversationError = "Dot memory must be 8 KiB or less."
		return
	}
	githubRepositoryIDs := append([]int64(nil), a.profileGitHubRepositoryIDs...)
	computerUseScopes := append([]string(nil), a.profileComputerUseScopes...)
	current, _ := findConversation(a.snapshot, a.selectedConversationID)
	if a.profileSpaceID != current.SpaceID || !a.profileGitHubEnabledDraft || !a.profileGitHubDisclosureAccepted {
		githubRepositoryIDs = nil
	}
	computerUseDisclosureAccepted := a.profileComputerUseDisclosureAccepted
	if a.profileSpaceID != current.SpaceID {
		computerUseScopes = nil
		computerUseDisclosureAccepted = false
	}
	computerUseChanged := !sameComputerUseScopes(current.ComputerUseScopes, computerUseScopes) ||
		current.ComputerUseModelDisclosureAccepted != computerUseDisclosureAccepted
	err := a.db.UpdateConversationProfileAndIntegrations(
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
		computerUseScopes,
		computerUseDisclosureAccepted,
	)
	if err != nil {
		a.conversationError = err.Error()
		a.adt.Error("Couldn't save the Dot profile", err.Error())
		return
	}
	if computerUseChanged && a.computerUseService != nil {
		a.computerUseService.StopAsync()
	}
	if err := a.db.SetConversationContextSummary(a.selectedConversationID, a.profileContextSummary); err != nil {
		a.conversationError = err.Error()
		a.adt.Error("Couldn't save chat memory", err.Error())
		a.refresh()
		return
	}
	a.selectedSpaceID = a.profileSpaceID
	a.conversationError = ""
	a.conversationDetailsOpen = false
	a.activityOpen = false
	a.adt.Success("Dot profile saved", "New replies will follow these instructions.")
	a.refresh()
}

func sameComputerUseScopes(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, scope := range left {
		if !containsID(right, scope) {
			return false
		}
	}
	return true
}

func (a *app) sendConversationMessage() {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.conversationError = "Choose a Dot before sending a message."
		return
	}
	if a.activeRunDotIDForConversation(conversation.ID) != "" {
		a.conversationError = "Wait for this Dot's reply or stop it before sending another message."
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
	a.chatMessages.ScrollToEnd()
	a.startRun(dot.ID)
}

func (a *app) stopConversationRun() {
	dotID := a.activeRunDotIDForConversation(a.selectedConversationID)
	if dotID == "" {
		return
	}
	a.cancelRunByID(dotID)
	a.adt.Info("Stopping the reply", "Your Dot will stop as soon as it can.")
}

func (a *app) retryConversationMessage(dotID string) {
	dot, ok := a.db.GetDot(dotID)
	if !ok || a.activeRunDotIDForConversation(dot.ConversationID) != "" {
		return
	}
	if err := a.db.SetStatus(dotID, task.Ready, "Retry requested by the user."); err != nil {
		a.conversationError = err.Error()
		return
	}
	a.refresh()
	a.startRun(dotID)
}
