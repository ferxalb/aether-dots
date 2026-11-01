package desktopui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func taskPanel(c *ui.Context, theme *ui.Theme, title string, content func()) {
	panel := ui.Column(c).FillWidth().Gap(10).Padding(13).Radius(13).
		Background(theme.Surface).Border(1, theme.Border)
	panel.Children(func() {
		ui.Text(c, title).FontSize(12).Bold().TextColor(theme.TextMuted)
		content()
	})
}

func taskScopePanel(c *ui.Context, theme *ui.Theme, sourceRoot, outputDir, mode, model string) {
	taskPanel(c, theme, "Local permissions", func() {
		taskScopeLine(c, theme, "Read only", sourceRoot)
		taskScopeLine(c, theme, "Write only", outputDir)
		if mode == "model" {
			if model == "" {
				model = "the model in Settings"
			}
			ui.Divider(c)
			ui.Text(c, fmt.Sprintf("Selected source text is sent to OpenAI using %s. The API key remains in system credentials or OPENAI_API_KEY.", model)).
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
	})
}

func taskScopeLine(c *ui.Context, theme *ui.Theme, permission, path string) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(10).Children(func() {
		ui.Text(c, permission).FontSize(11).Bold().TextColor(theme.TextMuted).Width(76)
		if path == "" {
			path = "No folder selected"
		}
		ui.Text(c, path).FontSize(12).FillWidth().Tooltip(path)
	})
}
