package desktopui

import (
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

const (
	workspaceSidebarWidth          = 252
	workspaceRailWidth             = 80
	workspaceHeaderHeight          = 44
	profileInspectorWidth          = 336
	profileInspectorMinWindowWidth = 900
)

func (a *app) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.startCreator() })
	theme := a.theme(c)
	latestDots := a.dotStatusByConversationID
	width, _ := c.Size()
	inspector := a.conversationDetailsOpen && a.selectedConversationID != "" &&
		!a.settingsOpen && !a.activityOpen && !a.approvals.open && width >= profileInspectorMinWindowWidth
	if !inspector {
		c.OnShortcut(ui.Cmd, ui.KeyK, func() { a.sidebarSearchInput.Focus(c) })
	}
	sidebarWidth := float32(workspaceSidebarWidth)
	if inspector {
		sidebarWidth = workspaceRailWidth
	}
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
		a.sidebar(c, theme, dragHeight, sidebarWidth, vibrant, inspector, latestDots)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.42))

		// Only the sidebar may be translucent; the content pane is always opaque.
		main := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Background(theme.Background)
		main.Children(func() {
			a.header(c, theme, latestDots, inspector)
			ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.34))

			switch {
			case a.settingsOpen:
				a.settingsView(c, theme)
			case a.activityOpen:
				a.activityView(c, theme)
			case a.approvals.open:
				a.githubApprovalsView(c, theme)
			case inspector || a.hasConversationChat():
				a.chatView(c, theme)
			case a.conversationDetailsOpen:
				a.profileContent(c, theme)
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
		if inspector {
			ui.Box(c).Width(1).FillHeight().Background(theme.Border)
			a.profileInspector(c, theme)
		}
		// Reserve the native window-control area at the left edge.
		ui.Box(c).Absolute().Top(0).Left(0).Width(workspaceRailWidth).Height(dragHeight).DragWindow()
	})
	toastConfig := ADTConfig{Left: sidebarWidth + 14, MaxWidth: 320}
	if a.settingsOpen {
		toastConfig.Position = ADTBottomRight
	}
	a.adt.Render(c, theme, toastConfig, root)
}

func (a *app) hasConversationChat() bool {
	if a.settingsOpen || a.activityOpen || a.approvals.open || a.conversationDetailsOpen || a.selectedDotID != "" {
		return false
	}
	_, ok := findConversation(a.snapshot, a.selectedConversationID)
	return ok
}

func (a *app) hasWelcomePage() bool {
	return !a.settingsOpen && !a.activityOpen && !a.approvals.open && !a.conversationDetailsOpen && a.selectedDotID == "" && a.selectedConversationID == ""
}

func (a *app) workspaceContent(c *ui.Context, theme *ui.Theme) {
	if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
		a.dotView(c, theme, dot)
		return
	}
	a.workspaceWelcomeView(c, theme)
}

func (a *app) header(c *ui.Context, theme *ui.Theme, latestDots map[string]conversationStatusSummary, inspector bool) {
	row := ui.Row(c).FillWidth().Height(workspaceHeaderHeight).Padding(0, 16).AlignItems(ui.Center).Gap(10).DragWindow()
	row.Children(func() {
		switch {
		case a.settingsOpen:
			ui.Text(c, "Settings").FontSize(13).Bold()
		case a.activityOpen:
			ui.Text(c, "Activity").FontSize(13).Bold()
		case a.approvals.open:
			ui.Text(c, "GitHub approvals").FontSize(13).Bold()
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
				profileAvatarForConversation(c, conversation, 22)
				ui.Text(c, conversation.Name).SingleLine().FontSize(13).Bold().
					MaxWidth(300).Shrink(1).MinWidth(0)
				if dot, ok := latestDots[conversation.ID]; ok {
					switch dot.status {
					case task.Running, task.Ready:
						ui.Text(c, "Working").FontSize(10).TextColor(theme.TextMuted)
					case task.Failed:
						ui.Text(c, "Needs attention").FontSize(10).TextColor(theme.Danger)
					}
				}
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
		} else if a.approvals.open {
			if ui.Button(c.Key("approvals-close"), "Back to workspace").Padding(7, 11).Clicked() {
				a.approvals.open = false
			}
		} else if inspector {
			if appIconButton(c, theme, "profile-back-to-chat", "Close Dot profile", iconSettings).
				Tooltip("Close Dot profile").Background(theme.SurfaceHover).Clicked() {
				a.conversationDetailsOpen = false
			}
		} else if a.conversationDetailsOpen {
			if appButton(c, theme, "profile-back-to-chat", "Back to chat", buttonGhost).Clicked() {
				a.conversationDetailsOpen = false
			}
		} else if a.selectedConversationID != "" {
			if appButton(c, theme, "conversation-customize", "Customize", buttonSecondary).
				Tooltip("Customize Dot profile").Clicked() {
				a.openConversationProfile()
			}
		}
	})
}
