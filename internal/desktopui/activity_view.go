package desktopui

import (
	"sort"
	"time"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) activityView(c *ui.Context, theme *ui.Theme) {
	items := append([]task.Dot(nil), a.snapshot.Dots...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	activeCount := 0
	for _, dot := range items {
		if dot.Status == task.Running || dot.Status == task.Ready {
			activeCount++
		}
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).MaxWidth(960).Gap(16).Padding(24, 28, 36, 28).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "Activity").FontSize(22).Bold()
					ui.Text(c, "See what each Dot is doing, stop individual runs, and review response speed.").
						FontSize(12).TextColor(theme.TextMuted)
				})
				ui.Spacer(c)
				if activeCount > 0 {
					ui.Badge(c, itoa(activeCount)+" active").TextColor(theme.Accent)
				}
			})
			if len(items) == 0 {
				ui.Column(c).FillWidth().Gap(6).Padding(22).Radius(14).Background(theme.Surface).Border(1, theme.Border).Children(func() {
					ui.Text(c, "No Dot activity yet").FontSize(14).Bold()
					ui.Text(c, "Send a message to a Dot and its run will appear here.").FontSize(12).TextColor(theme.TextMuted)
				})
				return
			}
			a.activityList.KeepOffset = true
			a.activityList.Key = func(index int) any { return items[index].ID }
			ui.List(c.Key("activity-dot-list"), &a.activityList, len(items)).
				FillWidth().MinHeight(280).Gap(10).
				Rows(func(row ui.ListRow) {
					activityCard(row.Context, theme, a, items[row.Index])
				})
		})
	})
}

func activityCard(c *ui.Context, theme *ui.Theme, a *app, dot task.Dot) {
	card := ui.Column(c.Key("activity-card-"+dot.ID)).FillWidth().MinWidth(0).Gap(9).Padding(15, 17).Radius(14).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, statusIcon(dot.Status)).Size(14, 14).TextColor(statusColor(theme, dot.Status))
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, dot.Name).SingleLine().FontSize(13).Bold().FillWidth()
				label := selectedSpaceName(a.snapshot, dot.SpaceID)
				if label == "" {
					label = "Personal"
				}
				ui.Text(c, label+" · "+dot.UpdatedAt.Local().Format("Jan 2, 3:04 PM")).
					FontSize(10).TextColor(theme.TextMuted)
			})
			ui.Badge(c, humanStatus(dot.Status)).TextColor(statusColor(theme, dot.Status))
		})
		if dot.Checkpoint != "" {
			ui.Text(c, compactProfileText(dot.Checkpoint, 220)).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		if dot.Metrics != nil {
			metrics := dot.Metrics
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				ui.Text(c, metrics.Provider).FontSize(10).TextColor(theme.TextMuted)
				ui.Text(c, "· "+formatElapsed(time.Duration(metrics.DurationMillis)*time.Millisecond)).FontSize(10).TextColor(theme.TextMuted)
				if metrics.FirstTokenLatencyMillis > 0 {
					ui.Text(c, "· first token "+formatElapsed(time.Duration(metrics.FirstTokenLatencyMillis)*time.Millisecond)).FontSize(10).TextColor(theme.TextMuted)
				}
				ui.Text(c, "· ~"+itoa(int(metrics.EstimatedInputTokens))+" in / ~"+itoa(int(metrics.EstimatedOutputTokens))+" out tokens").
					FontSize(10).TextColor(theme.TextMuted)
			})
		} else if dot.Status == task.Running {
			started := dot.CreatedAt
			ui.Text(c, "Running for "+formatElapsed(time.Since(started))).FontSize(10).TextColor(theme.TextMuted)
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if dot.ConversationID != "" {
				if ui.Button(c.Key("activity-open-chat-"+dot.ID), "Open chat").Padding(6, 9).Clicked() {
					a.activityOpen = false
					a.selectConversation(dot.ConversationID)
				}
			}
			ui.Spacer(c)
			if key := activeRunKeyForDot(a, dot); key != "" {
				if ui.Button(c.Key("activity-stop-"+dot.ID), "Stop").Padding(6, 9).Clicked() {
					a.cancelRunByID(key)
				}
			} else if dot.Status == task.Ready {
				if ui.Button(c.Key("activity-start-"+dot.ID), "Start").Padding(6, 9).Clicked() {
					a.startRun(dot.ID)
				}
			} else if dot.Status == task.Paused || dot.Status == task.Failed {
				if ui.Button(c.Key("activity-retry-"+dot.ID), "Retry").Padding(6, 9).Clicked() {
					a.retryConversationMessage(dot.ID)
				}
			}
		})
	})
}

func activeRunKeyForDot(a *app, dot task.Dot) string {
	if a.runCancels[dot.ID] != nil {
		return dot.ID
	}
	if dot.RoutineID != "" {
		key := "routine:" + dot.RoutineID
		if a.runCancels[key] != nil {
			return key
		}
	}
	return ""
}

func formatElapsed(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	switch {
	case value < time.Minute:
		return value.Round(time.Second).String()
	case value < time.Hour:
		return itoa(int(value/time.Minute)) + "m " + itoa(int(value%time.Minute/time.Second)) + "s"
	default:
		return itoa(int(value/time.Hour)) + "h " + itoa(int(value%time.Hour/time.Minute)) + "m"
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	i := len(buffer)
	for value > 0 {
		i--
		buffer[i] = digits[value%10]
		value /= 10
	}
	if negative {
		i--
		buffer[i] = '-'
	}
	return string(buffer[i:])
}
