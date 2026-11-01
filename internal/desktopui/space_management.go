package desktopui

import "strings"

func (a *app) openSpaceCreate() {
	a.spaceManagementOpen = true
	a.renameSpaceID = ""
	a.spaceDraft = ""
}

func (a *app) openSpaceRename(spaceID, name string) {
	a.spaceManagementOpen = true
	a.renameSpaceID = spaceID
	a.spaceDraft = name
}

func (a *app) saveSpaceDraft() {
	name := strings.TrimSpace(a.spaceDraft)
	if name == "" {
		a.conversationError = "Space name is required."
		return
	}
	if a.renameSpaceID != "" {
		if err := a.db.RenameSpace(a.renameSpaceID, name); err != nil {
			a.conversationError = err.Error()
			return
		}
		a.conversationError = ""
		a.message = "Space renamed."
	} else {
		space, err := a.db.CreateSpace(name)
		if err != nil {
			a.conversationError = err.Error()
			return
		}
		a.profileSpaceID = space.ID
		a.conversationError = ""
		a.message = "Space created."
	}
	a.refresh()
	a.closeSpaceManagement()
}

func (a *app) closeSpaceManagement() {
	a.spaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
}
