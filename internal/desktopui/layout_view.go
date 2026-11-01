package desktopui

import (
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	theme := a.theme(c)
	bar := c.TitleBar()
	dragHeight := max(bar.Height, 32)
	contentTop := max(dragHeight+14, 48)
	vibrant := c.Vibrancy() && a.prefs.Appearance == "System"
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	rootColor := theme.Background
	if vibrant {
		rootColor = ui.Transparent
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(rootColor).Children(func() {
		a.sidebar(c, theme, contentTop, vibrant)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.55))
		pane := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Margin(8).Radius(20).
			Background(paneColor(theme, vibrant)).Border(1, theme.Border.Alpha(0.55)).
			Shadow(0, 4, 24, 0, ui.RGBA(0, 0, 0, 0.12))
		pane.Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Gap(20).Padding(contentTop, 28, 28, 28).Children(func() {
					a.header(c, theme)
					if a.settingsOpen {
						a.settingsView(c, theme)
					} else if a.selectedRoutineID != "" {
						a.routineView(c, theme)
					} else if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
						a.dotView(c, theme, dot)
					} else {
						a.captureView(c, theme)
					}
				})
			})
		})
		// The hidden native title bar keeps the traffic lights and resize
		// affordances. This clear strip remains a direct window drag target.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(dragHeight).DragWindow()
	})
}

func (a *app) sidebar(c *ui.Context, theme *ui.Theme, top float32, vibrant bool) {
	width, inset := float32(248), float32(16)
	if a.sidebarCollapsed {
		width, inset = 72, 9
	}
	panel := ui.Column(c).Width(width).FillHeight().Gap(14).Padding(top, inset, 14, inset).
		Background(sidebarColor(theme, vibrant))
	panel.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			brand := ui.Box(c).Size(30, 30).Shrink(0).Radius(10).
				Gradient(theme.Accent, theme.AccentHover, 45).TextColor(theme.AccentText)
			brand.Children(func() { ui.Icon(c, iconSparkles).Size(17, 17) })
			if !a.sidebarCollapsed {
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, "AETHER").FontSize(14).Bold()
					ui.Text(c, "DOTS").FontSize(9).Bold().TextColor(theme.TextMuted)
				})
			}
		})

		if !a.sidebarCollapsed {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 4).Children(func() {
				sidebarSection(c, theme, iconFolder, "SPACES")
			})
		}
		for _, space := range a.snapshot.Spaces {
			button := ui.Button(c.Key("space-"+space.ID), "").FillWidth().Height(36).
				Padding(8, 10).Gap(9).Radius(10).Label(space.Name).Tooltip(space.Name)
			styleSidebarItem(button, theme, space.ID == a.selectedSpaceID)
			button.Children(func() {
				ui.Icon(c, iconFolder).Size(16, 16)
				if !a.sidebarCollapsed {
					ui.Text(c, space.Name).SingleLine().FontSize(13)
				}
			})
			if button.Clicked() {
				a.settingsOpen = false
				a.selectedSpaceID = space.ID
				a.selectedDotID = ""
				a.selectedRoutineID = ""
				a.refresh()
			}
		}
		if !a.sidebarCollapsed {
			ui.Row(c).Gap(6).Children(func() {
				ui.TextInput(c.Key("sidebar-space-draft"), &a.spaceDraft).Placeholder("Space name").Grow(1).MinWidth(0).Label("Space name")
				add := ui.Button(c.Key("sidebar-add-space"), "Add").Label("Add space")
				if add.Clicked() {
					space, err := a.db.CreateSpace(a.spaceDraft)
					if err != nil {
						a.message = err.Error()
					} else {
						a.selectedSpaceID = space.ID
						a.spaceDraft = ""
						a.message = "Space saved."
					}
					a.refresh()
				}
			})
		} else {
			add := ui.Button(c.Key("add-space"), "").FillWidth().Height(36).
				Label("Add space").Tooltip("Add space").Padding(8).Radius(10)
			add.Children(func() { ui.Icon(c, iconPlus).Size(16, 16) })
			if add.Clicked() {
				a.sidebarCollapsed = false
			}
		}

		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if !a.sidebarCollapsed {
				sidebarSection(c, theme, iconClock, "RECENT DOTS")
			} else {
				ui.Icon(c, iconClock).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Recent Dots")
			}
			ui.Spacer(c)
			newButton := ui.Button(c.Key("new-dot"), "").Height(30).Padding(5, 8).Radius(9).
				Label("New Dot").Tooltip("New Dot")
			if !a.sidebarCollapsed {
				newButton.Children(func() {
					ui.Icon(c, iconPlus).Size(15, 15)
					ui.Text(c, "New").FontSize(12)
				})
			} else {
				newButton.Width(30)
				newButton.Children(func() { ui.Icon(c, iconPlus).Size(16, 16) })
			}
			if newButton.Clicked() {
				a.beginCapture(false)
			}
		})

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				for i := len(a.snapshot.Dots) - 1; i >= 0; i-- {
					dot := a.snapshot.Dots[i]
					selected := dot.ID == a.selectedDotID
					button := ui.Button(c.Key("dot-"+dot.ID), "").FillWidth().Height(36).
						Padding(8, 10).Gap(9).Radius(10).Label(dot.Name).Tooltip(dot.Name + " · " + humanStatus(dot.Status))
					styleSidebarItem(button, theme, selected)
					button.Children(func() {
						ui.Icon(c, statusIcon(dot.Status)).Size(16, 16).TextColor(statusColor(theme, dot.Status))
						if !a.sidebarCollapsed {
							ui.Text(c, dot.Name).SingleLine().FontSize(12)
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
				if len(a.snapshot.Routines) > 0 {
					ui.Divider(c)
					if !a.sidebarCollapsed {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 4).Children(func() {
							sidebarSection(c, theme, iconRepeat, "ROUTINES")
						})
					} else {
						ui.Icon(c, iconRepeat).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Routines")
					}
					for _, routine := range a.snapshot.Routines {
						selected := routine.ID == a.selectedRoutineID
						button := ui.Button(c.Key("routine-"+routine.ID), "").FillWidth().Height(36).
							Padding(8, 10).Gap(9).Radius(10).Label(routine.Name).Tooltip(routine.Name)
						styleSidebarItem(button, theme, selected)
						button.Children(func() {
							ui.Icon(c, iconRepeat).Size(16, 16)
							if !a.sidebarCollapsed {
								ui.Text(c, routine.Name).SingleLine().FontSize(12)
							}
						})
						if button.Clicked() {
							a.settingsOpen = false
							a.selectedRoutineID = routine.ID
							a.selectedDotID = ""
							a.selectedSpaceID = ""
							a.routineName = routine.Name
							a.routineObjective = routine.Objective
							a.refresh()
						}
					}
				}
			})
		})

		if a.sidebarCollapsed {
			settings := ui.Button(c.Key("open-settings"), "").FillWidth().Height(36).
				Label("Settings").Tooltip("Settings").Padding(8).Radius(10)
			styleSidebarItem(settings, theme, a.settingsOpen)
			settings.Children(func() { ui.Icon(c, iconSettings).Size(16, 16) })
			if settings.Clicked() {
				a.openSettings()
			}
			ui.Icon(c, iconSparkles).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Local runtime · this device")
		} else {
			settings := ui.Button(c.Key("open-settings"), "").FillWidth().Height(36).
				Label("Settings").Tooltip("Settings").Padding(8, 10).Gap(9).Radius(10)
			styleSidebarItem(settings, theme, a.settingsOpen)
			settings.Children(func() {
				ui.Icon(c, iconSettings).Size(16, 16)
				ui.Text(c, "Settings").FontSize(12)
			})
			if settings.Clicked() {
				a.openSettings()
			}
			ui.Text(c, "Local runtime · this device").FontSize(11).TextColor(theme.TextMuted)
		}
	})
}

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		toggle := ui.Button(c.Key("toggle-sidebar"), "").Size(36, 36).Padding(8).Radius(10).
			Label("Toggle sidebar").Tooltip("Toggle sidebar")
		icon := iconPanelOpen
		label := "Collapse sidebar"
		if a.sidebarCollapsed {
			icon, label = iconPanelClosed, "Expand sidebar"
		}
		toggle.Label(label).Tooltip(label)
		if toggle.Hovered() {
			toggle.Background(theme.SurfaceHover)
		}
		toggle.Children(func() { ui.Icon(c, icon).Size(18, 18) })
		if toggle.Clicked() {
			a.sidebarCollapsed = !a.sidebarCollapsed
		}
		title, subtitle := "AETHER Dots", "One outcome. One permission scope. One resumable history."
		if a.settingsOpen {
			title, subtitle = "Settings", "Make AETHER Dots feel at home on this Mac."
		}
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(19).Bold()
			ui.Text(c, subtitle).FontSize(12).TextColor(theme.TextMuted)
		})
		ui.Spacer(c)
		if !a.settingsOpen && (a.selectedRoutineID != "" || a.selectedDotID != "") {
			newCapture := ui.Button(c.Key("header-new-dot"), "").Padding(9, 12).Gap(7).
				Label("Create a new Dot").Tooltip("Create a new Dot")
			newCapture.Children(func() {
				ui.Icon(c, iconPlus).Size(15, 15)
				ui.Text(c, "New Dot").FontSize(12)
			})
			if newCapture.Clicked() {
				a.beginCapture(false)
			}
		}
		ui.Badge(c, "⌘ ⇧ SPACE · CAPTURE")
	})
	if a.message != "" {
		status := ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(10, 12).Radius(11).
			Background(theme.Accent.Alpha(0.1)).Border(1, theme.Accent.Alpha(0.18))
		status.Children(func() {
			ui.Icon(c, iconSparkles).Size(15, 15).TextColor(theme.Accent)
			ui.Text(c, a.message).FontSize(12).TextColor(theme.Text).FillWidth()
		})
	}
}
