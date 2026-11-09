package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.startCreator() })
	theme := a.theme(c)
	latestDots := a.dotStatusByConversationID
	bar := c.TitleBar()
	dragHeight := max(bar.Height, 44)
	vibrant := c.Vibrancy() && a.prefs.Appearance == "System"
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	rootColor := theme.Background
	if vibrant {
		rootColor = ui.Transparent
	}

	root := ui.Row(c).Fill().AlignItems(ui.Stretch).Background(rootColor)
	root.Children(func() {
		a.sidebar(c, theme, dragHeight, vibrant, latestDots)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.42))

		// Only the sidebar may be translucent; the content pane is always opaque.
		main := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Background(theme.Background)
		main.Children(func() {
			a.header(c, theme, latestDots)
			ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.34))

			switch {
			case a.settingsOpen:
				a.settingsView(c, theme)
			case a.activityOpen:
				a.activityView(c, theme)
			case a.hasConversationChat():
				a.chatView(c, theme)
			case a.hasWelcomePage():
				a.workspaceWelcomeView(c, theme)
			default:
				ui.Scroll(c).Grow(1).FillWidth().Children(func() {
					ui.Column(c).FillWidth().MinWidth(0).Gap(18).Padding(22, 26, 30, 26).Children(func() {
						a.workspaceContent(c, theme)
					})
				})
			}
		})
		// The main header is also draggable; this separate area covers the
		// sidebar's clear titlebar band without blocking its controls.
		ui.Box(c).Absolute().Top(0).Left(0).Width(272).Height(dragHeight).DragWindow()
	})
	toastConfig := ADTConfig{Left: 286, MaxWidth: 320}
	if a.settingsOpen {
		toastConfig.Position = ADTBottomRight
	}
	a.adt.Render(c, theme, toastConfig, root)
}

func (a *app) hasConversationChat() bool {
	if a.settingsOpen || a.activityOpen || a.conversationDetailsOpen || a.selectedDotID != "" {
		return false
	}
	_, ok := findConversation(a.snapshot, a.selectedConversationID)
	return ok
}

func (a *app) hasWelcomePage() bool {
	return !a.settingsOpen && !a.activityOpen && !a.conversationDetailsOpen && a.selectedDotID == "" && a.selectedConversationID == ""
}

func (a *app) workspaceContent(c *ui.Context, theme *ui.Theme) {
	if a.settingsOpen {
		a.settingsView(c, theme)
		return
	}
	if a.activityOpen {
		a.activityView(c, theme)
		return
	}
	if a.conversationDetailsOpen {
		a.conversationDetailsView(c, theme)
		return
	}
	if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
		a.dotView(c, theme, dot)
		return
	}
	a.workspaceWelcomeView(c, theme)
}

func (a *app) header(c *ui.Context, theme *ui.Theme, latestDots map[string]conversationStatusSummary) {
	row := ui.Row(c).FillWidth().Height(52).Padding(0, 20).AlignItems(ui.Center).Gap(10).DragWindow()
	row.Children(func() {
		switch {
		case a.settingsOpen:
			ui.Text(c, "Settings").FontSize(13).Bold()
		case a.activityOpen:
			ui.Text(c, "Activity").FontSize(13).Bold()
		case a.selectedDotID != "":
			if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
				a.conversationAvatar(c, dot.Name, dot.ID, 30)
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, dot.Name).SingleLine().FontSize(12).Bold().MaxWidth(240).MinWidth(0)
					ui.Text(c, "Older task").FontSize(10).TextColor(theme.TextMuted)
				})
			}
		case a.selectedConversationID != "":
			if conversation, ok := findConversation(a.snapshot, a.selectedConversationID); ok {
				profileAvatarForConversation(c, conversation, 32)
				ui.Column(c).Gap(2).MinWidth(0).Children(func() {
					ui.Text(c, conversation.Name).SingleLine().FontSize(13).Bold().
						MaxWidth(300).Shrink(1).MinWidth(0)
					status := "Ready"
					if dot, ok := latestDots[conversation.ID]; ok {
						status = humanStatus(dot.status)
					}
					ui.Text(c, status).FontSize(10).TextColor(theme.TextMuted)
				})
			}
		default:
			ui.Text(c, "AETHER Dots").FontSize(13).Bold()
		}

		ui.Spacer(c)
		if a.settingsOpen {
			if ui.Button(c.Key("settings-close"), "Done").Padding(7, 11).Clicked() {
				a.settingsOpen = false
			}
		} else if a.activityOpen {
			if ui.Button(c.Key("activity-close"), "Back to workspace").Padding(7, 11).Clicked() {
				a.activityOpen = false
			}
		} else if a.conversationDetailsOpen {
			if ui.Button(c.Key("profile-back-to-chat"), "Back to chat").Padding(7, 11).Clicked() {
				a.conversationDetailsOpen = false
			}
		} else if a.selectedConversationID != "" {
			customize := ui.Button(c.Key("conversation-customize"), "Customize").Padding(7, 11).
				Label("Customize Dot profile")
			if customize.Hovered() {
				customize.Background(theme.SurfaceHover)
			}
			if customize.Clicked() {
				a.openConversationProfile()
			}
		}
	})
}
