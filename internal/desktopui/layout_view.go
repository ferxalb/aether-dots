package desktopui

import (
	"math"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.startCreator() })
	theme := a.theme(c)
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

	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(rootColor).Children(func() {
		a.sidebar(c, theme, dragHeight, vibrant)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.42))

		main := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Background(rootColor)
		main.Children(func() {
			a.header(c, theme)
			ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.34))

			switch {
			case a.settingsOpen:
				a.settingsView(c, theme)
			case a.hasConversationChat():
				a.chatView(c, theme)
			case a.hasWelcomePage():
				a.workspaceWelcomeView(c, theme)
			default:
				ui.Scroll(c).Grow(1).FillWidth().Children(func() {
					ui.Column(c).FillWidth().MinWidth(0).Gap(18).Padding(22, 26, 30, 26).Children(func() {
						a.statusMessage(c, theme)
						a.workspaceContent(c, theme)
					})
				})
			}
		})
		// The main header is also draggable; this separate area covers the
		// sidebar's clear titlebar band without blocking its controls.
		ui.Box(c).Absolute().Top(0).Left(0).Width(272).Height(dragHeight).DragWindow()
	})
}

func (a *app) hasConversationChat() bool {
	if a.settingsOpen || a.conversationDetailsOpen || a.selectedDotID != "" {
		return false
	}
	_, ok := findConversation(a.snapshot, a.selectedConversationID)
	return ok
}

func (a *app) hasWelcomePage() bool {
	return !a.settingsOpen && !a.conversationDetailsOpen && a.selectedDotID == "" && a.selectedConversationID == ""
}

func (a *app) workspaceContent(c *ui.Context, theme *ui.Theme) {
	if a.settingsOpen {
		a.settingsView(c, theme)
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

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	row := ui.Row(c).FillWidth().Height(52).Padding(0, 20).AlignItems(ui.Center).Gap(10).DragWindow()
	row.Children(func() {
		switch {
		case a.settingsOpen:
			ui.Text(c, "Settings").FontSize(13).Bold()
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
					if dotStatus, ok := latestConversationStatus(a.snapshot, conversation.ID); ok {
						status = humanStatus(dotStatus)
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

func (a *app) statusMessage(c *ui.Context, theme *ui.Theme) {
	if a.message == "" {
		return
	}
	status := ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(8, 11).Radius(9).
		Background(theme.Accent.Alpha(0.08)).Border(1, theme.Accent.Alpha(0.14))
	status.Children(func() {
		ui.Icon(c, iconSparkles).Size(13, 13).TextColor(theme.Accent)
		ui.Text(c, a.message).FontSize(11).TextColor(theme.Text).FillWidth()
	})
}

func (a *app) followChatIfAtBottom() {
	if a.chatScroll.MaxY-a.chatScroll.Y <= 48 {
		a.chatScroll.ScrollTo(0, math.MaxFloat32, ui.Smooth)
	}
}

func latestConversationDot(db task.Database, conversationID string) (task.Dot, bool) {
	var latest task.Dot
	found := false
	for _, dot := range db.Dots {
		if dot.ConversationID != conversationID {
			continue
		}
		if !found || dot.UpdatedAt.After(latest.UpdatedAt) {
			latest, found = dot, true
		}
	}
	return latest, found
}
