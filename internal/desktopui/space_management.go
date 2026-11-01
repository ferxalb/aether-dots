package desktopui

import "strings"

func (a *app) openSpaceCreate() {
	a.creatorSpaceManagementOpen = true
	a.renameSpaceID = ""
	a.spaceDraft = ""
}

func (a *app) openSpaceRename(spaceID, name string) {
	a.creatorSpaceManagementOpen = true
	a.renameSpaceID = spaceID
	a.spaceDraft = name
}

func (a *app) saveSpaceDraft() {
	name := strings.TrimSpace(a.spaceDraft)
	if a.renameSpaceID != "" {
		if err := a.db.RenameSpace(a.renameSpaceID, name); err != nil {
			a.message = err.Error()
			return
		}
		a.message = "Space renamed."
	} else {
		space, err := a.db.CreateSpace(name)
		if err != nil {
			a.message = err.Error()
			return
		}
		a.selectedSpaceID = space.ID
		a.selectedConversationID = ""
		a.selectedDotID = ""
		a.selectedRoutineID = ""
		a.creatorOpen = true
		a.settingsOpen = false
		a.message = "Space created."
	}
	a.closeSpaceManagement()
	a.refresh()
}

func (a *app) closeSpaceManagement() {
	a.creatorSpaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
}
