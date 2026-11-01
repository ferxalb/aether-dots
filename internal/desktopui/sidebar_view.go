package desktopui

import (
	"sort"
	"strings"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

var (
	iconEdit = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4Z"/></svg>`))
)

func (a *app) sidebar(c *ui.Context, theme *ui.Theme, top float32, vibrant bool) {
	width := float32(258)
	if a.sidebarCollapsed {
		width = 70
	}
	panel := ui.Column(c).Width(width).FillHeight().Gap(12).
		Padding(top, 12, 12, 12).Background(sidebarColor(theme, vibrant))
	panel.Children(func() {
		a.sidebarBrand(c, theme)
		a.newDotButton(c, theme)

		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().MinWidth(0).Gap(14).Padding(0, 0, 2, 0).Children(func() {
				a.spacesSection(c, theme)
				ui.Divider(c)
				a.historySection(c, theme)
				a.routinesSection(c, theme)
			})
		})

		a.sidebarFooter(c, theme)
	})
}

func (a *app) sidebarBrand(c *ui.Context, theme *ui.Theme) {
	row := ui.Row(c).FillWidth().Height(34).AlignItems(ui.Center).Gap(9)
	if a.sidebarCollapsed {
		row.Justify(ui.Center)
	}
	row.Children(func() {
		brand := ui.Box(c).Size(28, 28).Shrink(0).Radius(9).
			Gradient(theme.Accent, theme.AccentHover, 45).TextColor(theme.AccentText)
		brand.Children(func() { ui.Icon(c, iconSparkles).Size(16, 16) })
		if !a.sidebarCollapsed {
			ui.Column(c).Gap(0).Children(func() {
				ui.Text(c, "AETHER Dots").FontSize(13).Bold()
				ui.Text(c, "LOCAL WORKSPACE").FontSize(9).Bold().TextColor(theme.TextMuted)
			})
		}
	})
}

func (a *app) newDotButton(c *ui.Context, theme *ui.Theme) {
	label := "New Dot"
	button := ui.Button(c.Key("sidebar-new-dot"), "").FillWidth().Height(38).
		Padding(9, 11).Gap(9).Radius(10).Label(label).Tooltip(label).
		Background(theme.Accent).Border(1, theme.Accent).TextColor(theme.AccentText)
	if a.sidebarCollapsed {
		button.Center()
	}
	if button.Hovered() {
		button.Background(theme.AccentHover).Border(1, theme.AccentHover)
	}
	button.Children(func() {
		ui.Icon(c, iconPlus).Size(16, 16)
		if !a.sidebarCollapsed {
			ui.Text(c, label).FontSize(12).Bold()
		}
	})
	if button.Clicked() {
		a.beginCapture(false)
	}
}

func (a *app) spacesSection(c *ui.Context, theme *ui.Theme) {
	header := ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Gap(6)
	header.Children(func() {
		if a.sidebarCollapsed {
			ui.Icon(c, iconFolder).Size(15, 15).TextColor(theme.TextMuted).Tooltip("Spaces")
			ui.Spacer(c)
		} else {
			sidebarSection(c, theme, iconFolder, "Spaces")
			ui.Spacer(c)
		}
		add := ui.Button(c.Key("sidebar-add-space-toggle"), "").Size(24, 24).Padding(5).Radius(7).
			Label("Add a space").Tooltip("Add a space")
		if add.Hovered() {
			add.Background(theme.SurfaceHover)
		}
		add.Children(func() { ui.Icon(c, iconPlus).Size(14, 14) })
		if add.Clicked() {
			a.sidebarCollapsed = false
			a.spaceManagementOpen = !a.spaceManagementOpen || a.renameSpaceID != ""
			a.renameSpaceID = ""
			a.spaceDraft = ""
		}
	})

	for _, space := range a.snapshot.Spaces {
		a.spaceRow(c, theme, space)
	}
	if a.spaceManagementOpen && !a.sidebarCollapsed {
		a.spaceManagementForm(c, theme)
	}
}

func (a *app) spaceRow(c *ui.Context, theme *ui.Theme, space task.Space) {
	selected := space.ID == a.selectedSpaceID
	row := ui.Row(c).FillWidth().Height(33).AlignItems(ui.Center).Gap(2)
	row.Children(func() {
		button := ui.Button(c.Key("space-"+space.ID), "").Grow(1).MinWidth(0).Height(32).
			Padding(7, 9).Gap(8).Radius(8).Label(space.Name).Tooltip(space.Name)
		if a.sidebarCollapsed {
			button.Center()
		}
		styleSidebarItem(button, theme, selected)
		button.Children(func() {
			ui.Icon(c, iconFolder).Size(15, 15).TextColor(theme.TextMuted)
			if !a.sidebarCollapsed {
				ui.Text(c, space.Name).SingleLine().FontSize(12).FillWidth()
			}
		})
		if button.Clicked() {
			a.settingsOpen = false
			a.selectedSpaceID = space.ID
			a.selectedDotID = ""
			a.selectedRoutineID = ""
			a.refresh()
		}
		if !a.sidebarCollapsed && (selected || button.Hovered()) {
			rename := ui.Button(c.Key("rename-space-"+space.ID), "").Size(27, 27).Padding(6).Radius(7).
				Label("Rename " + space.Name).Tooltip("Rename " + space.Name)
			if rename.Hovered() {
				rename.Background(theme.SurfaceHover)
			}
			rename.Children(func() { ui.Icon(c, iconEdit).Size(13, 13).TextColor(theme.TextMuted) })
			if rename.Clicked() {
				a.spaceManagementOpen = true
				a.renameSpaceID = space.ID
				a.spaceDraft = space.Name
			}
		}
	})
}

func (a *app) spaceManagementForm(c *ui.Context, theme *ui.Theme) {
	form := ui.Column(c).FillWidth().Gap(7).Padding(8).Radius(9).
		Background(theme.Background.Alpha(0.48)).Border(1, theme.Border.Alpha(0.6))
	form.Children(func() {
		title, actionLabel, inputLabel, placeholder := "Add a space", "Create", "Space name", "A name for this space"
		if a.renameSpaceID != "" {
			title, actionLabel, inputLabel, placeholder = "Rename space", "Save", "New space name", "Enter a new name"
		}
		ui.Text(c, title).FontSize(11).Bold().TextColor(theme.TextMuted)
		ui.TextInput(c.Key("space-management-name"), &a.spaceDraft).
			FillWidth().MinWidth(0).Placeholder(placeholder).Label(inputLabel)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			save := ui.PrimaryButton(c.Key("space-management-save"), actionLabel).Grow(1).Label(actionLabel)
			if save.Clicked() {
				a.saveSpaceDraft()
			}
			cancel := ui.Button(c.Key("space-management-cancel"), "Cancel").Label("Cancel")
			if cancel.Clicked() {
				a.closeSpaceManagement()
			}
		})
	})
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
		a.selectedDotID = ""
		a.selectedRoutineID = ""
		a.settingsOpen = false
		a.message = "Space created."
	}
	a.closeSpaceManagement()
	a.refresh()
}

func (a *app) closeSpaceManagement() {
	a.spaceManagementOpen = false
	a.renameSpaceID = ""
	a.spaceDraft = ""
}

func (a *app) historySection(c *ui.Context, theme *ui.Theme) {
	header := ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Gap(6)
	if a.sidebarCollapsed {
		header.Justify(ui.Center)
	}
	header.Children(func() {
		if a.sidebarCollapsed {
			ui.Icon(c, iconClock).Size(15, 15).TextColor(theme.TextMuted).
				Tooltip("Recent Dots · " + selectedSpaceName(a.snapshot, a.selectedSpaceID))
		} else {
			sidebarSection(c, theme, iconClock, "Recent Dots")
			ui.Spacer(c)
			ui.Text(c, selectedSpaceName(a.snapshot, a.selectedSpaceID)).FontSize(10).
				TextColor(theme.TextMuted).SingleLine().MaxWidth(105).Shrink(1).MinWidth(0)
		}
	})

	dots := recentDots(a.snapshot, a.selectedSpaceID)
	if len(dots) == 0 {
		if !a.sidebarCollapsed {
			ui.Text(c, "No Dots in this Space yet").FontSize(11).TextColor(theme.TextMuted).Padding(5, 4)
		}
		return
	}
	for _, dot := range dots {
		a.historyRow(c, theme, dot)
	}
}

func recentDots(db task.Database, spaceID string) []task.Dot {
	dots := make([]task.Dot, 0, len(db.Dots))
	for _, dot := range db.Dots {
		if spaceID == "" || dot.SpaceID == spaceID {
			dots = append(dots, dot)
		}
	}
	sort.SliceStable(dots, func(i, j int) bool { return dots[i].UpdatedAt.After(dots[j].UpdatedAt) })
	return dots
}

func (a *app) historyRow(c *ui.Context, theme *ui.Theme, dot task.Dot) {
	selected := dot.ID == a.selectedDotID
	button := ui.Button(c.Key("dot-"+dot.ID), "").FillWidth().Height(42).
		Padding(7, 9).Gap(8).Radius(8).Label(dot.Name).
		Tooltip(dot.Name + " · " + humanStatus(dot.Status))
	if a.sidebarCollapsed {
		button.Center()
	}
	styleSidebarItem(button, theme, selected)
	button.Children(func() {
		ui.Icon(c, statusIcon(dot.Status)).Size(15, 15).TextColor(statusColor(theme, dot.Status))
		if !a.sidebarCollapsed {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, dot.Name).SingleLine().FontSize(11).FillWidth()
				ui.Text(c, humanStatus(dot.Status)).SingleLine().FontSize(9).TextColor(theme.TextMuted)
			})
		}
	})
	if button.Clicked() {
		a.settingsOpen = false
		a.selectedDotID = dot.ID
		a.selectedRoutineID = ""
		a.selectedSpaceID = dot.SpaceID
		a.refresh()
	}
}

func (a *app) routinesSection(c *ui.Context, theme *ui.Theme) {
	if len(a.snapshot.Routines) == 0 {
		return
	}
	ui.Divider(c)
	if a.sidebarCollapsed {
		ui.Row(c).FillWidth().Height(24).Justify(ui.Center).Children(func() {
			ui.Icon(c, iconRepeat).Size(15, 15).TextColor(theme.TextMuted).Tooltip("Routines")
		})
	} else {
		ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Gap(6).Children(func() {
			sidebarSection(c, theme, iconRepeat, "Routines")
		})
	}
	for _, routine := range a.snapshot.Routines {
		button := ui.Button(c.Key("routine-"+routine.ID), "").FillWidth().Height(35).
			Padding(8, 9).Gap(8).Radius(8).Label(routine.Name).Tooltip("Routine · " + routine.Name)
		if a.sidebarCollapsed {
			button.Center()
		}
		styleSidebarItem(button, theme, routine.ID == a.selectedRoutineID)
		button.Children(func() {
			ui.Icon(c, iconRepeat).Size(15, 15).TextColor(theme.TextMuted)
			if !a.sidebarCollapsed {
				ui.Text(c, routine.Name).SingleLine().FontSize(11).FillWidth()
			}
		})
		if button.Clicked() {
			a.settingsOpen = false
			a.selectedRoutineID = routine.ID
			a.selectedDotID = ""
			a.routineName = routine.Name
			a.routineObjective = routine.Objective
			a.refresh()
		}
	}
}

func (a *app) sidebarFooter(c *ui.Context, theme *ui.Theme) {
	footer := ui.Column(c).FillWidth().Gap(4)
	footer.Children(func() {
		settings := ui.Button(c.Key("open-settings"), "").FillWidth().Height(36).
			Padding(8, 9).Gap(9).Radius(9).Label("Settings").Tooltip("Settings")
		if a.sidebarCollapsed {
			settings.Center()
		}
		styleSidebarItem(settings, theme, a.settingsOpen)
		settings.Children(func() {
			ui.Icon(c, iconSettings).Size(16, 16).TextColor(theme.TextMuted)
			if !a.sidebarCollapsed {
				ui.Text(c, "Settings").FontSize(12)
			}
		})
		if settings.Clicked() {
			a.openSettings()
		}
		if !a.sidebarCollapsed {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Padding(4, 2).Children(func() {
				ui.Icon(c, iconSparkles).Size(12, 12).TextColor(theme.TextMuted)
				ui.Text(c, "Local runtime · this device").FontSize(10).TextColor(theme.TextMuted)
			})
		}
	})
}
