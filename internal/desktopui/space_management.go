package desktopui

import "strings"

func (a *app) openSpaceCreate() {
	a.spaceManagementOpen = true
	a.renameSpaceID = ""
	a.spaceDraft = ""
	a.spaceContextDraft = ""
	a.spaceContextTargetID = ""
}

func (a *app) openSpaceRename(spaceID, name string) {
	a.spaceManagementOpen = true
	a.renameSpaceID = spaceID
	a.spaceDraft = name
	a.spaceContextTargetID = spaceID
	a.spaceContextDraft = ""
	for _, space := range a.snapshot.Spaces {
		if space.ID == spaceID {
			a.spaceContextDraft = space.Context
			break
		}
	}
}

func (a *app) saveSpaceDraft() {
	name := strings.TrimSpace(a.spaceDraft)
	if name == "" {
		a.conversationError = "Space name is required."
		return
	}
	if a.renameSpaceID != "" {
		if err := a.db.UpdateSpace(a.renameSpaceID, name, a.spaceContextDraft); err != nil {
			a.conversationError = err.Error()
			return
		}
		a.conversationError = ""
		a.adt.Success("Space renamed", "The new name is saved.")
	} else {
		space, err := a.db.CreateSpaceWithContext(name, a.spaceContextDraft)
		if err != nil {
			a.conversationError = err.Error()
			return
		}
		a.profileSpaceID = space.ID
		a.conversationError = ""
		a.adt.Success("Space created", "Your new space is ready.")
	}
	a.refresh()
	a.closeSpaceManagement()
}

func (a *app) closeSpaceManagement() {
	a.spaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
	a.spaceContextDraft = ""
	a.spaceContextTargetID = ""
}

func (a *app) saveSpaceContextDraft() {
	spaceID := a.spaceContextTargetID
	name := selectedSpaceName(a.snapshot, spaceID)
	if spaceID == "" || spaceID != a.profileSpaceID || name == "" {
		a.conversationError = "Select and load a Space before saving its shared notes."
		return
	}
	if err := a.db.UpdateSpace(spaceID, name, a.spaceContextDraft); err != nil {
		a.conversationError = err.Error()
		a.adt.Error("Couldn't save Space notes", err.Error())
		return
	}
	a.conversationError = ""
	a.refresh()
	a.adt.Success("Space notes saved", "Dots in this Space will use these notes as shared context.")
}
