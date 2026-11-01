package desktopui

import (
	"fmt"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) executionDisclosure() string {
	return executionDisclosureFor(a.mode, a.prefs.Model)
}

func creatorExecutionDisclosureFor(mode string) string {
	if mode == "model" {
		return "Sends your request, this Dot’s purpose, recent chat context, and selected source text to OpenAI. Older context may be omitted."
	}
	return "Local extraction uses this Dot’s purpose and current request only. It does not reason over earlier chat or send source text off this Mac."
}

func executionDisclosureFor(mode, model string) string {
	if mode == "model" {
		if model == "" {
			model = "the model in Settings"
		}
		return fmt.Sprintf("Sends your request, this Dot’s purpose, selected source text, and up to 6 recent chat messages (16 KiB total) to OpenAI using %s. Older messages may be omitted and long entries excerpted. The key stays in system credentials or OPENAI_API_KEY.", model)
	}
	return "Fixed marker extraction uses this Dot’s purpose and current request only; it does not reason over earlier chat. No model is called and source text stays on this Mac."
}

func executionSelect(c *ui.Context, theme *ui.Theme, selected *string) {
	type choice struct {
		value string
		label string
	}
	choices := []choice{{value: "local", label: "Local extraction"}, {value: "model", label: "OpenAI model"}}
	selectedLabel := "Local extraction"
	if *selected == "model" {
		selectedLabel = "OpenAI model"
	}
	selectMode := ui.SelectBase(c.Key("creator-execution-mode"), selected)
	selectMode.Trigger.Width(174).Padding(8, 10).Gap(8).Radius(9).
		Background(theme.Surface).Border(1, theme.Border).Label("Execution mode").
		Children(func() {
			ui.Text(c, selectedLabel).FontSize(12).FillWidth()
			ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
		})
	selectMode.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(theme.Surface).Border(1, theme.Border).
			Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
		for _, option := range choices {
			item := selectMode.Item(option.value).FillWidth().Padding(9, 10).Radius(7)
			if item.Highlighted() {
				item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
			}
			item.Children(func() { ui.Text(c, option.label).FontSize(12) })
		}
	})
}

func spaceSelect(c *ui.Context, theme *ui.Theme, selected *string, spaces []task.Space) {
	selectedName := "Select a space"
	for _, space := range spaces {
		if space.ID == *selected {
			selectedName = space.Name
			break
		}
	}
	selectSpace := ui.SelectBase(c.Key("creator-space"), selected)
	selectSpace.Trigger.MinWidth(190).MaxWidth(260).Padding(8, 10).Gap(8).Radius(9).
		Background(theme.Surface).Border(1, theme.Border).Label("Space").
		Children(func() {
			ui.Icon(c, iconFolder).Size(15, 15).TextColor(theme.TextMuted)
			ui.Text(c, selectedName).FontSize(12).FillWidth().SingleLine()
			ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
		})
	selectSpace.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(theme.Surface).Border(1, theme.Border).
			Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
		for _, option := range spaces {
			item := selectSpace.Item(option.ID).FillWidth().Padding(9, 10).Radius(7)
			if item.Highlighted() {
				item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
			}
			item.Children(func() { ui.Text(c, option.Name).FontSize(12) })
		}
	})
}

func folderPermissionRow(c *ui.Context, theme *ui.Theme, permission, key, path, purpose string, action func()) {
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(5, 8).Radius(10).
		Background(theme.Surface).Border(1, theme.Border)
	row.Children(func() {
		ui.Icon(c, iconFolder).Size(16, 16).TextColor(theme.Accent)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, permission).FontSize(12).Bold()
			value := path
			if value == "" {
				value = "Choose a folder"
			}
			ui.Text(c, value).FontSize(10).TextColor(theme.TextMuted).SingleLine().FillWidth().Tooltip(path)
		})
		choose := ui.Button(c.Key("choose-"+key), "Choose…").Padding(7, 9).Radius(8).
			Label(purpose).Tooltip(purpose)
		if choose.Clicked() {
			action()
		}
	})
}
