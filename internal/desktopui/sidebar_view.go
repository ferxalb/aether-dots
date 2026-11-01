package desktopui

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

var iconSearch = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 4.5 4.5"/></svg>`))

func (a *app) sidebar(c *ui.Context, theme *ui.Theme, top float32, vibrant bool) {
	panel := ui.Column(c).Width(280).FillHeight().Gap(11).
		Padding(top, 10, 12, 10).Background(sidebarColor(theme, vibrant, a.prefs.Palette))
	panel.Children(func() {
		createRow := ui.Row(c).FillWidth().Height(25).AlignItems(ui.Center)
		createRow.Children(func() {
			ui.Spacer(c)
			create := ui.Button(c.Key("sidebar-create-dot"), "").Size(28, 26).Padding(5).Radius(7).
				Label("Create a Dot").Tooltip("Create a Dot")
			if create.Hovered() {
				create.Background(theme.SurfaceHover)
			}
			create.Children(func() { ui.Icon(c, iconPlus).Size(17, 17) })
			if create.Clicked() {
				a.startCreator()
			}
		})

		search := ui.Row(c).FillWidth().Height(34).AlignItems(ui.Center).Gap(7).
			Padding(0, 9).Radius(9).Background(theme.Surface).Border(1, theme.Border)
		search.Children(func() {
			ui.Icon(c, iconSearch).Size(14, 14).TextColor(theme.TextMuted)
			input := ui.TextInput(c.Key("sidebar-search"), &a.searchQuery).
				FillWidth().MinWidth(0).Placeholder("Search").Label("Search Dots")
			input.Background(ui.Transparent).Border(1, ui.Transparent).Padding(5, 0)
		})

		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().MinWidth(0).Gap(4).Children(func() {
				a.dotList(c, theme)
			})
		})

		a.sidebarFooter(c, theme)
	})
}

func (a *app) dotList(c *ui.Context, theme *ui.Theme) {
	query := strings.TrimSpace(strings.ToLower(a.searchQuery))
	conversations := recentConversations(a.snapshot, query)
	legacyDots := recentLegacyDots(a.snapshot, query)
	if len(conversations) == 0 && len(legacyDots) == 0 {
		message := "No Dots yet"
		if query != "" {
			message = "No Dots match your search"
		}
		ui.Text(c, message).FontSize(11).TextColor(theme.TextMuted).Padding(7, 8).FillWidth()
		return
	}
	for _, conversation := range conversations {
		a.conversationRow(c, theme, conversation)
	}
	if len(legacyDots) > 0 {
		if len(conversations) > 0 {
			ui.Divider(c)
		}
		ui.Text(c, "Older Dots").FontSize(10).Bold().TextColor(theme.TextMuted).Padding(4, 6)
		for _, dot := range legacyDots {
			a.legacyDotRow(c, theme, dot)
		}
	}
}

func recentConversations(db task.Database, query string) []task.Conversation {
	items := make([]task.Conversation, 0, len(db.Conversations))
	for _, conversation := range db.Conversations {
		spaceName := selectedSpaceName(db, conversation.SpaceID)
		searchable := strings.ToLower(strings.Join([]string{
			conversation.Name, boundedSidebarText(conversation.Purpose), spaceName, conversationPreview(conversation),
		}, " "))
		if query != "" && !strings.Contains(searchable, query) {
			continue
		}
		items = append(items, conversation)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func recentLegacyDots(db task.Database, query string) []task.Dot {
	items := make([]task.Dot, 0, len(db.Dots))
	for _, dot := range db.Dots {
		if conversationExists(db, dot.ConversationID) {
			continue
		}
		spaceName := selectedSpaceName(db, dot.SpaceID)
		searchable := strings.ToLower(strings.Join([]string{
			dot.Name, boundedSidebarText(dot.Objective), spaceName, legacyDotPreview(dot), string(dot.Status),
		}, " "))
		if query != "" && !strings.Contains(searchable, query) {
			continue
		}
		items = append(items, dot)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func conversationExists(db task.Database, id string) bool {
	if id == "" {
		return false
	}
	for _, conversation := range db.Conversations {
		if conversation.ID == id {
			return true
		}
	}
	return false
}

func (a *app) conversationRow(c *ui.Context, theme *ui.Theme, conversation task.Conversation) {
	selected := conversation.ID == a.selectedConversationID
	button := ui.Button(c.Key("conversation-"+conversation.ID), "").FillWidth().Height(56).
		Padding(7, 8).Gap(9).Radius(10).Label(conversation.Name).
		Tooltip(conversation.Name + " · " + selectedSpaceName(a.snapshot, conversation.SpaceID))
	styleConversationRow(button, theme, selected)
	button.Children(func() {
		a.conversationAvatar(c, conversation.Name, conversation.ID, 34)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, conversation.Name).SingleLine().FontSize(12).Bold().FillWidth().MinWidth(0)
				ui.Text(c, sidebarTime(conversation.UpdatedAt)).Width(58).Shrink(0).
					SingleLine().FontSize(9).TextColor(theme.TextMuted)
			})
			preview := conversationPreview(conversation)
			if status, ok := latestConversationStatus(a.snapshot, conversation.ID); ok {
				switch status {
				case task.Running:
					preview = "Working · " + preview
				case task.Failed:
					preview = "Needs attention · " + preview
				case task.Paused:
					preview = "Paused · " + preview
				case task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
					preview = "Waiting · " + preview
				}
			}
			ui.Text(c, preview).SingleLine().FontSize(10).TextColor(theme.TextMuted).
				FillWidth().Tooltip(conversationPreview(conversation))
		})
	})
	if button.Clicked() {
		a.selectConversation(conversation.ID)
	}
}

func (a *app) legacyDotRow(c *ui.Context, theme *ui.Theme, dot task.Dot) {
	selected := dot.ID == a.selectedDotID
	button := ui.Button(c.Key("legacy-dot-"+dot.ID), "").FillWidth().Height(56).
		Padding(7, 8).Gap(9).Radius(10).Label(dot.Name).
		Tooltip(dot.Name + " · " + selectedSpaceName(a.snapshot, dot.SpaceID))
	styleConversationRow(button, theme, selected)
	button.Children(func() {
		a.conversationAvatar(c, dot.Name, dot.ID, 34)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, dot.Name).SingleLine().FontSize(12).Bold().FillWidth().MinWidth(0)
				ui.Text(c, sidebarTime(dot.UpdatedAt)).Width(58).Shrink(0).
					SingleLine().FontSize(9).TextColor(theme.TextMuted)
			})
			ui.Text(c, legacyDotPreview(dot)).SingleLine().FontSize(10).TextColor(theme.TextMuted).
				FillWidth().Tooltip(legacyDotPreview(dot))
		})
	})
	if button.Clicked() {
		a.rememberCurrentConversationState()
		a.selectedDotID = dot.ID
		a.selectedConversationID = ""
		a.messageDraft = ""
		a.chatScroll = ui.ScrollState{}
		a.selectedRoutineID = ""
		a.routinesOpen = false
		a.creatorOpen = false
		a.conversationDetailsOpen = false
		a.selectedSpaceID = dot.SpaceID
		a.settingsOpen = false
		a.refresh()
	}
}

func (a *app) conversationAvatar(c *ui.Context, name, id string, size float32) {
	avatar := ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).
		Background(avatarColor(id)).TextColor(ui.Hex("#FFFFFF"))
	avatar.Center().Children(func() {
		ui.Text(c, avatarInitial(name)).FontSize(13).Bold().TextColor(ui.Hex("#FFFFFF"))
	})
}

func avatarInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "D"
}

func avatarColor(id string) ui.Color {
	colors := []string{"#7458D8", "#268A73", "#D27C31", "#3F7DE2", "#B04763", "#37839A"}
	hash := 0
	for i := 0; i < len(id); i++ {
		hash = (hash*31 + int(id[i])) % len(colors)
	}
	return ui.Hex(colors[hash])
}

func styleConversationRow(button ui.Element, theme *ui.Theme, selected bool) {
	button.Background(ui.Transparent).Border(1, ui.Transparent).TextColor(theme.Text)
	if selected {
		button.Background(theme.SurfaceHover).Border(1, theme.Border.Alpha(0.7))
	} else if button.Hovered() {
		button.Background(theme.SurfaceHover.Alpha(0.72))
	}
}

func conversationPreview(conversation task.Conversation) string {
	if len(conversation.Messages) > 0 {
		return boundedSidebarText(conversation.Messages[len(conversation.Messages)-1].Content)
	}
	return boundedSidebarText(conversation.Purpose)
}

func legacyDotPreview(dot task.Dot) string {
	if len(dot.Events) > 0 {
		return boundedSidebarText(dot.Events[len(dot.Events)-1].Message)
	}
	if dot.Status != task.Done {
		return humanStatus(dot.Status)
	}
	return boundedSidebarText(dot.Objective)
}

func boundedSidebarText(value string) string {
	const maxBytes, maxRunes = 768, 180
	truncated := false
	if len(value) > maxBytes {
		end := maxBytes
		for end > 0 && !utf8.RuneStart(value[end]) {
			end--
		}
		value = value[:end]
		truncated = true
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes-1]
		value = string(runes)
		truncated = true
	}
	if truncated {
		return strings.TrimRight(value, " .") + "…"
	}
	return value
}

func latestConversationStatus(db task.Database, conversationID string) (task.Status, bool) {
	dots := make([]task.Dot, 0)
	for _, dot := range db.Dots {
		if dot.ConversationID == conversationID {
			dots = append(dots, dot)
		}
	}
	if len(dots) == 0 {
		return "", false
	}
	sort.SliceStable(dots, func(i, j int) bool { return dots[i].UpdatedAt.After(dots[j].UpdatedAt) })
	return dots[0].Status, true
}

func sidebarTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	now, local := time.Now().Local(), at.Local()
	sameDay := now.Year() == local.Year() && now.YearDay() == local.YearDay()
	if sameDay {
		return local.Format("3:04 PM")
	}
	yesterday := now.AddDate(0, 0, -1)
	if yesterday.Year() == local.Year() && yesterday.YearDay() == local.YearDay() {
		return "Yesterday"
	}
	if now.Sub(local) < 7*24*time.Hour {
		return local.Format("Mon")
	}
	return local.Format("Jan 2")
}

func (a *app) sidebarFooter(c *ui.Context, theme *ui.Theme) {
	footer := ui.Column(c).FillWidth().Gap(4)
	footer.Children(func() {
		routines := ui.Button(c.Key("open-routines"), "").FillWidth().Height(36).
			Padding(8, 10).Gap(9).Radius(9).Label("Routines").Tooltip("Routines")
		styleSidebarItem(routines, theme, a.routinesOpen || a.selectedRoutineID != "")
		routines.Children(func() {
			ui.Icon(c, iconRepeat).Size(16, 16).TextColor(theme.TextMuted)
			ui.Text(c, "Routines").FontSize(12)
		})
		if routines.Clicked() {
			a.routinesOpen = true
			a.selectedRoutineID = ""
			a.routineName = ""
			a.routineObjective = ""
			a.settingsOpen = false
			a.creatorOpen = false
			a.conversationDetailsOpen = false
		}

		settings := ui.Button(c.Key("open-settings"), "").FillWidth().Height(38).
			Padding(8, 10).Gap(9).Radius(9).Label("Settings").Tooltip("Settings")
		styleSidebarItem(settings, theme, a.settingsOpen)
		settings.Children(func() {
			ui.Icon(c, iconSettings).Size(16, 16).TextColor(theme.TextMuted)
			ui.Text(c, "Settings").FontSize(12)
		})
		if settings.Clicked() {
			a.openSettings()
			a.creatorOpen = false
			a.routinesOpen = false
			a.conversationDetailsOpen = false
		}
		ui.Text(c, "Local runtime · this device").FontSize(10).TextColor(theme.TextMuted).Padding(2, 4)
	})
}
