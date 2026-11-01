package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) modelOptionsSettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(13).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "Generation controls").FontSize(15).Bold()
			ui.Text(c, "Tune reasoning, response length, and recovery for both chat and routine runs.").FontSize(12).TextColor(theme.TextMuted)
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			modelChoice(c, theme, "settings-reasoning", "Reasoning effort", &a.prefs.ReasoningEffort,
				[]string{"none", "minimal", "low", "medium", "high", "xhigh", "max"})
			modelChoice(c, theme, "settings-verbosity", "Response detail", &a.prefs.TextVerbosity,
				[]string{"low", "medium", "high"})
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			modelChoice(c, theme, "settings-output-tokens", "Maximum output tokens", &a.prefs.MaxOutputTokens,
				[]string{"256", "512", "1024", "2048", "4096", "8192", "16384", "32768", "65536"})
			modelChoice(c, theme, "settings-retries", "Automatic retries", &a.prefs.MaxRetries,
				[]string{"0", "1", "2", "3", "4", "5"})
		})
		modelChoice(c, theme, "settings-timeout", "Request timeout (seconds)", &a.prefs.TimeoutSeconds,
			[]string{"30", "45", "60", "90", "120", "180", "300"})
		ui.Text(c, "Higher reasoning and output limits can use more API tokens. Retry count applies to temporary API errors.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
	})
}

func modelChoice(c *ui.Context, theme *ui.Theme, key, label string, value *string, options []string) {
	field := ui.Column(c).Grow(1).FillWidth().MinWidth(0).Gap(5)
	field.Children(func() {
		ui.Text(c, label).FontSize(11).Bold()
		selectOption := ui.SelectBase(c.Key(key), value)
		selectOption.Trigger.FillWidth().MinWidth(0).Padding(8, 10).Radius(9).
			Background(theme.Background).Border(1, theme.Border).
			Children(func() {
				ui.Text(c, *value).FontSize(12).FillWidth().SingleLine()
				ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
			})
		selectOption.Popup(func(panel ui.Element) {
			panel.Padding(4).Radius(10).Background(theme.Surface).Border(1, theme.Border).
				Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
			for _, option := range options {
				item := selectOption.Item(option).FillWidth().Padding(9, 10).Radius(7)
				if item.Highlighted() || option == *value {
					item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
				}
				item.Children(func() { ui.Text(c, option).FontSize(12) })
			}
		})
	})
}
