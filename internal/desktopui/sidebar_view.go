package desktopui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

var iconSearch = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 4.5 4.5"/></svg>`))

func (a *app) sidebar(c *ui.Context, theme *ui.Theme, top, width float32, vibrant, compact bool, latestDots map[string]conversationStatusSummary) {
	panel := ui.Column(c).Width(width).Shrink(0).FillHeight().Gap(8).
		Padding(0, 10, 10, 10).Background(sidebarColor(theme, vibrant, a.prefs.Palette))
	panel.Children(func() {
		if compact {
			ui.Box(c).Height(top - 8).Shrink(0)
		} else {
			ui.Row(c.Key("sidebar-heading")).FillWidth().Height(top - 8).Shrink(0).
				AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Spacer(c)
				appIconButton(c, theme, "sidebar-create-dot", "New Dot", iconPlus).
					Tooltip("Create a new Dot · ⌘N").OnClick(a.startCreator)
			})
			a.sidebarSearch(c, theme)
		}

		a.dotList(c, theme, compact, latestDots)
		if compact {
			ui.Row(c.Key("rail-create")).FillWidth().Center().Children(func() {
				if appIconButton(c, theme, "sidebar-create-dot", "New Dot", iconPlus).
					Tooltip("Create a new Dot · ⌘N").Clicked() {
					a.startCreator()
				}
			})
		}
		a.sidebarFooter(c, theme, compact)
	})
}

func (a *app) sidebarSearch(c *ui.Context, theme *ui.Theme) {
	border := theme.Border
	if a.sidebarSearchInput.Focused(c) {
		border = theme.TextMuted
	}
	ui.Row(c.Key("dot-search-field")).FillWidth().Height(32).AlignItems(ui.Center).Gap(7).
		Padding(0, 9).Radius(workspaceControlRadius).Border(1, border).Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(theme.TextMuted)
		ui.TextInputBase(c.Key("sidebar-search"), &a.searchQuery).
			FillWidth().MinWidth(0).Placeholder("Search Dots").Label("Search Dots").
			Padding(5, 0).FocusRing(false).
			Bind(&a.sidebarSearchInput)
	})
}

func (a *app) dotList(c *ui.Context, theme *ui.Theme, compact bool, latestDots map[string]conversationStatusSummary) {
	query := strings.TrimSpace(strings.ToLower(a.searchQuery))
	if compact {
		query = ""
	}
	if a.sidebarResults == nil || a.sidebarResults.query != query {
		a.sidebarResults = &sidebarResults{
			query:         query,
			conversations: recentConversations(a.snapshot, query),
			legacyDots:    recentLegacyDots(a.snapshot, query),
		}
	}
	conversations, legacyDots := a.sidebarResults.conversations, a.sidebarResults.legacyDots
	if len(conversations) == 0 && len(legacyDots) == 0 {
		message := "No Dots yet"
		if query != "" {
			message = "No Dots match your search"
		}
		ui.Text(c, message).FontSize(11).TextColor(theme.TextMuted).Padding(7, 8).FillWidth().Grow(1)
		return
	}

	conversationCount := len(conversations)
	legacyStart := conversationCount
	if !compact && len(legacyDots) > 0 {
		legacyStart++
	}
	listKey := "workspace-dot-list"
	if compact {
		listKey = "workspace-dot-rail"
	}
	ui.List(c.Key(listKey), nil, legacyStart+len(legacyDots)).
		Grow(1).FillWidth().MinHeight(0).Gap(3).
		ItemKey(func(i int) any {
			switch {
			case i < conversationCount:
				return "conversation-" + conversations[i].ID
			case i < legacyStart:
				return "older-tasks-heading"
			default:
				return "legacy-dot-" + legacyDots[i-legacyStart].ID
			}
		}).
		Rows(func(row ui.ListRow) {
			switch {
			case row.Index < conversationCount:
				a.conversationRow(row.Context, theme, conversations[row.Index], compact, latestDots)
			case row.Index < legacyStart:
				ui.Text(row.Context, "OLDER TASKS").FontSize(9).Bold().TextColor(theme.TextMuted).Padding(6, 7)
			default:
				a.legacyDotRow(row.Context, theme, legacyDots[row.Index-legacyStart], compact)
			}
		})
}

// sidebarResults belongs to the current snapshot and normalized filter.
type sidebarResults struct {
	query         string
	conversations []task.Conversation
	legacyDots    []task.Dot
}

func recentConversations(db task.Database, query string) []task.Conversation {
	items := make([]task.Conversation, 0, len(db.Conversations))
	for _, conversation := range db.Conversations {
		if query != "" {
			searchable := strings.ToLower(strings.Join([]string{
				conversation.Name, boundedSidebarText(conversation.Purpose), selectedSpaceName(db, conversation.SpaceID),
				conversationPreview(conversation),
			}, " "))
			if !strings.Contains(searchable, query) {
				continue
			}
		}
		items = append(items, conversation)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func recentLegacyDots(db task.Database, query string) []task.Dot {
	conversationIDs := make(map[string]struct{}, len(db.Conversations))
	for _, conversation := range db.Conversations {
		conversationIDs[conversation.ID] = struct{}{}
	}
	items := make([]task.Dot, 0, len(db.Dots))
	for _, dot := range db.Dots {
		if dot.ConversationID != "" {
			if _, exists := conversationIDs[dot.ConversationID]; exists {
				continue
			}
		}
		if query != "" {
			searchable := strings.ToLower(strings.Join([]string{
				dot.Name, boundedSidebarText(dot.Objective), selectedSpaceName(db, dot.SpaceID), legacyDotPreview(dot), string(dot.Status),
			}, " "))
			if !strings.Contains(searchable, query) {
				continue
			}
		}
		items = append(items, dot)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func (a *app) conversationRow(c *ui.Context, theme *ui.Theme, conversation task.Conversation, compact bool, latestDots map[string]conversationStatusSummary) {
	preview := ""
	if !compact {
		preview = conversationPreview(conversation)
	}
	if dot, ok := latestDots[conversation.ID]; ok {
		switch dot.status {
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
	workspaceDotRow(c, theme, "conversation-"+conversation.ID, conversation.Name, preview, conversation.UpdatedAt,
		compact, conversation.ID == a.selectedConversationID, func() {
			profileAvatarForConversation(c, conversation, 30)
		}).OnClick(func() {
		a.selectConversation(conversation.ID)
	})
}

func (a *app) legacyDotRow(c *ui.Context, theme *ui.Theme, dot task.Dot, compact bool) {
	workspaceDotRow(c, theme, "legacy-dot-"+dot.ID, dot.Name, legacyDotPreview(dot), dot.UpdatedAt,
		compact, dot.ID == a.selectedDotID, func() {
			a.conversationAvatar(c, dot.Name, dot.ID, 30)
		}).OnClick(func() {
		a.rememberCurrentConversationState()
		a.selectedDotID = dot.ID
		a.selectedConversationID = ""
		a.messageDraft = ""
		a.chatMessages = ui.ListState{FollowEnd: true}
		a.conversationDetailsOpen = false
		a.selectedSpaceID = dot.SpaceID
		a.settingsOpen = false
		a.activityOpen = false
		a.approvals.open = false
		a.refresh()
	})
}

func workspaceDotRow(c *ui.Context, theme *ui.Theme, key, name, preview string, updated time.Time, compact, selected bool, avatar func()) ui.Element {
	button := ui.Button(c.Key(key), "").FillWidth().Height(49).
		Padding(6, 8).Gap(8).Radius(workspaceControlRadius).Label(name).Tooltip(name)
	styleConversationRow(button, theme, selected)
	if compact {
		button.Height(42).Radius(workspaceControlRadius).Center().Children(avatar)
		return button
	}
	button.Children(func() {
		avatar()
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, name).SingleLine().FontSize(13).Grow(1).MinWidth(0)
				ui.Text(c, sidebarTime(updated)).Shrink(0).SingleLine().FontSize(10).TextColor(theme.TextMuted)
			})
			ui.Text(c, preview).SingleLine().FontSize(11).TextColor(theme.TextMuted).
				FillWidth().Tooltip(preview)
		})
	})
	return button
}

func (a *app) conversationAvatar(c *ui.Context, name, id string, size float32) {
	background := avatarColor(id)
	ink := contrastInk(background)
	avatar := ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).
		Background(background).TextColor(ink)
	avatar.Center().Children(func() {
		ui.Text(c, avatarInitial(name)).FontSize(13).Bold().TextColor(ink)
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
	if strings.TrimSpace(conversation.Purpose) != "" {
		return boundedSidebarText(conversation.Purpose)
	}
	return "Ready for a prompt"
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

func (a *app) sidebarFooter(c *ui.Context, theme *ui.Theme, compact bool) {
	ui.Column(c.Key("sidebar-footer")).FillWidth().Gap(3).Children(func() {
		if a.snapshot.GitHubConnected || len(a.snapshot.GitHubProposals) > 0 {
			sidebarAction(c, theme, "open-approvals", "GitHub approvals", iconCheck,
				compact, a.approvals.open, a.pendingApprovalCount()).OnClick(a.openApprovals)
		}
		activeCount := 0
		for _, dot := range a.snapshot.Dots {
			if dot.Status == task.Running || dot.Status == task.Ready {
				activeCount++
			}
		}
		sidebarAction(c, theme, "open-activity", "Activity", iconClock, compact, a.activityOpen, activeCount).OnClick(func() {
			a.rememberCurrentConversationState()
			a.activityOpen = true
			a.approvals.open = false
			a.settingsOpen = false
			a.conversationDetailsOpen = false
			a.selectedDotID = ""
		})
		sidebarAction(c, theme, "open-settings", "Settings", iconSettings, compact, a.settingsOpen, 0).OnClick(func() {
			a.openSettings()
			a.conversationDetailsOpen = false
		})
	})
}

func sidebarAction(c *ui.Context, theme *ui.Theme, key, label string, icon *ui.SVG, compact, selected bool, count int) ui.Element {
	tooltip := label
	if count > 0 {
		tooltip += fmt.Sprintf(" (%d)", count)
	}
	button := ui.Button(c.Key(key), "").FillWidth().Height(32).
		Padding(6, 9).Gap(8).Radius(workspaceControlRadius).Label(label).Tooltip(tooltip)
	styleSidebarItem(button, theme, selected)
	if compact {
		button.Center()
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(theme.TextMuted)
		if !compact {
			ui.Text(c, label).FontSize(11)
			ui.Spacer(c)
		}
		if count > 0 {
			badge := fmt.Sprint(count)
			if compact && count > 99 {
				badge = "99+"
			}
			ui.Badge(c, badge).TextColor(theme.Accent)
		}
	})
	return button
}
