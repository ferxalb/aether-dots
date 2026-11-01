package desktopui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) captureView(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().Padding(4, 0, 0, 0).Children(func() {
		ui.Spacer(c)
		content := ui.Column(c).FillWidth().MaxWidth(760).MinWidth(0).Gap(9)
		content.Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "New Dot").FontSize(18).Bold()
				ui.Text(c, "Describe the outcome and choose the local folders AETHER may use.").FontSize(12).TextColor(theme.TextMuted).FillWidth()
			})
			composer := ui.Column(c).FillWidth().Gap(6).Padding(10).Radius(14).
				Background(theme.Surface).Border(1, theme.Border)
			composer.Children(func() {
				ui.Field(c, "Outcome", func() {
					ui.TextAreaBase(c.Key("capture-outcome"), &a.objective).
						Placeholder("Describe the result you want, such as a concise brief or review.").
						MinHeight(60).FillWidth()
				})
			})
			if composer.FocusWithin() {
				composer.BorderColor(theme.Focus)
			}

			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Column(c).Gap(3).Children(func() {
					ui.Text(c, "Local access").FontSize(13).Bold()
					ui.Text(c, "AETHER reads only the source folder and writes only to the output folder.").FontSize(11).TextColor(theme.TextMuted).FillWidth()
				})
				folderPermissionRow(c, theme, "Read", "source", a.sourceRoot, "Choose a source folder", func() { a.pickFolder(true) })
				folderPermissionRow(c, theme, "Write", "output", a.outputDir, "Choose an output folder", func() { a.pickFolder(false) })
			})

			ui.Column(c).FillWidth().Gap(5).Children(func() {
				ui.Text(c, "Execution").FontSize(13).Bold()
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(11).Children(func() {
					executionSelect(c, theme, &a.mode)
					ui.Text(c, a.executionDisclosure()).FontSize(11).TextColor(theme.TextMuted).FillWidth()
				})
			})

			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Save this Dot now; choose Run when you’re ready.").FontSize(11).TextColor(theme.TextMuted).Grow(1).MinWidth(0).FillWidth()
				create := ui.PrimaryButton(c.Key("create-dot"), "").Gap(8).Padding(9, 14).
					Label("Create Dot").Tooltip("Save this Dot")
				create.Children(func() {
					ui.Icon(c, iconPlus).Size(15, 15)
					ui.Text(c, "Create Dot").FontSize(12).Bold()
				})
				create.Disabled(strings.TrimSpace(a.objective) == "" || a.sourceRoot == "" || a.outputDir == "")
				if create.Clicked() {
					a.createDot()
				}
			})
		})
		ui.Spacer(c)
	})
}

func (a *app) executionDisclosure() string {
	if a.mode == "model" {
		model := a.prefs.Model
		if model == "" {
			model = "the model in Settings"
		}
		return fmt.Sprintf("Selected source text is sent to OpenAI using %s. The key stays in system credentials or OPENAI_API_KEY.", model)
	}
	return "Uses fixed local extraction. No model is called and source text stays on this Mac."
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
	selectMode := ui.SelectBase(c.Key("capture-execution-mode"), selected)
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
			item.Children(func() {
				ui.Text(c, option.label).FontSize(12)
			})
		}
	})
}

func folderPermissionRow(c *ui.Context, theme *ui.Theme, permission, key, path, purpose string, action func()) {
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(9).Padding(9, 10).Radius(11).
		Background(theme.Surface).Border(1, theme.Border)
	row.Children(func() {
		ui.Icon(c, iconFolder).Size(16, 16).TextColor(theme.Accent)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, permission).FontSize(12).Bold()
			value := path
			if value == "" {
				value = "Choose a folder"
			}
			ui.Text(c, value).FontSize(11).TextColor(theme.TextMuted).FillWidth().Tooltip(path)
		})
		choose := ui.Button(c.Key("choose-"+key), "Choose…").Padding(7, 9).Radius(8).
			Label(purpose).Tooltip(purpose)
		if choose.Clicked() {
			action()
		}
	})
}
