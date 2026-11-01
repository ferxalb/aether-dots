package desktopui

import (
	"math"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
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
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.4))

		main := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Background(rootColor)
		main.Children(func() {
			// Keep the native title-bar and traffic-light region clear.
			ui.Box(c).FillWidth().Height(dragHeight)
			a.header(c, theme)
			ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.28))

			switch {
			case a.hasConversationChat():
				a.chatView(c, theme)
			case a.hasWelcomePage():
				a.workspaceWelcomeView(c, theme)
			default:
				ui.Scroll(c).Grow(1).FillWidth().Children(func() {
					ui.Column(c).FillWidth().MinWidth(0).Gap(18).Padding(18, 24, 32, 24).Children(func() {
						a.statusMessage(c, theme)
						a.workspaceContent(c, theme)
					})
				})
			}
		})

		// Only the top inset drags the window; the toolbar remains interactive.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(dragHeight).DragWindow()
	})
}

func (a *app) hasConversationChat() bool {
	if a.settingsOpen || a.creatorOpen || a.routinesOpen || a.conversationDetailsOpen || a.selectedRoutineID != "" || a.selectedDotID != "" {
		return false
	}
	_, ok := findConversation(a.snapshot, a.selectedConversationID)
	return ok
}

func (a *app) hasWelcomePage() bool {
	return !a.settingsOpen && !a.creatorOpen && !a.routinesOpen && !a.conversationDetailsOpen &&
		a.selectedRoutineID == "" && a.selectedDotID == "" && a.selectedConversationID == ""
}

func (a *app) workspaceContent(c *ui.Context, theme *ui.Theme) {
	if a.settingsOpen {
		a.settingsView(c, theme)
		return
	}
	if a.routinesOpen {
		if a.selectedRoutineID != "" {
			a.routineView(c, theme)
		} else {
			a.routinesListView(c, theme)
		}
		return
	}
	if a.selectedRoutineID != "" {
		a.routineView(c, theme)
		return
	}
	if a.creatorOpen {
		a.creatorView(c, theme)
		return
	}
	if a.conversationDetailsOpen {
		if _, ok := findConversation(a.snapshot, a.selectedConversationID); ok {
			a.conversationDetailsView(c, theme)
			return
		}
	}
	if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
		a.dotView(c, theme, dot)
		return
	}
	a.workspaceWelcomeView(c, theme)
}

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	row := ui.Row(c).FillWidth().Height(48).Padding(0, 18).AlignItems(ui.Center).Gap(10)
	row.Children(func() {
		switch {
		case a.settingsOpen:
			ui.Text(c, "Settings").FontSize(13).Bold()
		case a.routinesOpen:
			ui.Text(c, "Routines").FontSize(13).Bold()
		case a.selectedRoutineID != "":
			ui.Text(c, "Routines").FontSize(13).Bold()
		case a.creatorOpen:
			ui.Text(c, "New Dot").FontSize(13).Bold()
		case a.selectedDotID != "":
			if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
				a.conversationAvatar(c, dot.Name, dot.ID, 30)
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, dot.Name).SingleLine().FontSize(12).Bold().MaxWidth(240).MinWidth(0)
					ui.Text(c, humanStatus(dot.Status)).FontSize(10).TextColor(theme.TextMuted)
				})
			}
		case a.selectedConversationID != "":
			if conversation, ok := findConversation(a.snapshot, a.selectedConversationID); ok {
				a.conversationAvatar(c, conversation.Name, conversation.ID, 30)
				ui.Column(c).Gap(1).MinWidth(0).Children(func() {
					ui.Text(c, conversation.Name).SingleLine().FontSize(12).Bold().
						MaxWidth(250).Shrink(1).MinWidth(0)
					status := "Ready"
					if dotStatus, ok := latestConversationStatus(a.snapshot, conversation.ID); ok {
						status = humanStatus(dotStatus)
					}
					ui.Text(c, status).FontSize(10).TextColor(theme.TextMuted)
				})
			}
		default:
			ui.Text(c, "Dots").FontSize(13).Bold()
		}

		ui.Spacer(c)
		if a.routinesOpen {
			label := "Back to Dots"
			if a.selectedRoutineID != "" {
				label = "Back to routines"
			} else if a.selectedConversationID != "" {
				label = "Back to chat"
			} else if a.selectedDotID != "" {
				label = "Back to Dot"
			}
			back := ui.Button(c.Key("routines-back"), label).Padding(7, 10).Radius(8).Label(label)
			if back.Hovered() {
				back.Background(theme.SurfaceHover)
			}
			if back.Clicked() {
				if a.selectedRoutineID != "" {
					a.selectedRoutineID = ""
					a.routineName = ""
					a.routineObjective = ""
				} else {
					a.routinesOpen = false
				}
			}
		} else if a.selectedRoutineID != "" && a.selectedConversationID != "" && !a.settingsOpen && !a.creatorOpen {
			back := ui.Button(c.Key("routine-back-to-details"), "Back to details").
				Padding(7, 10).Radius(8).Label("Back to details")
			if back.Hovered() {
				back.Background(theme.SurfaceHover)
			}
			if back.Clicked() {
				a.selectedRoutineID = ""
				a.conversationDetailsOpen = true
			}
		} else if a.selectedConversationID != "" && !a.settingsOpen && !a.creatorOpen &&
			a.selectedDotID == "" && a.selectedRoutineID == "" {
			label, tooltip := "Details", "Conversation details"
			if a.conversationDetailsOpen {
				label, tooltip = "Back to chat", "Return to chat"
			}
			details := ui.Button(c.Key("conversation-details"), label).Padding(7, 10).
				Radius(8).Label(label).Tooltip(tooltip)
			if details.Hovered() {
				details.Background(theme.SurfaceHover)
			}
			if details.Clicked() {
				a.conversationDetailsOpen = !a.conversationDetailsOpen
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

// followChatIfAtBottom keeps newly completed work in view while preserving a
// reader's place when they have scrolled back through earlier messages.
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
