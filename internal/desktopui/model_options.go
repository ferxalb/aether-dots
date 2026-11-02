package desktopui

import (
	"strconv"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func (a *app) modelOptionsSettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(13).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "Response behavior").FontSize(15).Bold()
			ui.Text(c, "Most people can leave these at their defaults. Changes apply to chat and scheduled replies.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth()
		})
		if a.prefs.Provider == string(model.ProviderOpenAI) {
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort,
					[]string{"none", "minimal", "low", "medium", "high", "xhigh", "max"})
				modelChoice(c, theme, "settings-verbosity", "Reply detail", &a.prefs.TextVerbosity,
					[]string{"low", "medium", "high"})
			})
		} else {
			ui.Text(c, "Thinking effort and reply detail are controlled by this provider's model and are not sent as unsupported settings.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			modelChoice(c, theme, "settings-output-tokens", "Maximum reply length", &a.prefs.MaxOutputTokens,
				[]string{"256", "512", "1024", "2048", "4096", "8192", "16384", "32768", "65536"})
			modelChoice(c, theme, "settings-retries", "Retries for temporary errors", &a.prefs.MaxRetries,
				[]string{"0", "1", "2", "3", "4", "5"})
		})
		modelChoice(c, theme, "settings-timeout", "Stop waiting after", &a.prefs.TimeoutSeconds,
			[]string{"30", "45", "60", "90", "120", "180", "300"})
		ui.Text(c, "More detailed replies can take longer and may cost more.").
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
				ui.Text(c, modelChoiceLabel(key, *value)).FontSize(12).FillWidth().SingleLine()
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
				item.Children(func() { ui.Text(c, modelChoiceLabel(key, option)).FontSize(12) })
			}
		})
	})
}

func modelChoiceLabel(key, value string) string {
	switch key {
	case "settings-reasoning":
		switch value {
		case "none":
			return "Off"
		case "minimal":
			return "Minimal"
		case "low":
			return "Light"
		case "medium":
			return "Balanced"
		case "high":
			return "Deep"
		case "xhigh":
			return "Very deep"
		case "max":
			return "Maximum"
		}
	case "settings-verbosity":
		switch value {
		case "low":
			return "Brief"
		case "medium":
			return "Balanced"
		case "high":
			return "Detailed"
		}
	case "settings-output-tokens":
		return value + " tokens"
	case "settings-retries":
		if value == "1" {
			return "1 retry"
		}
		return value + " retries"
	case "settings-timeout":
		seconds := integerPreference(value, 0)
		if seconds%60 == 0 {
			minutes := seconds / 60
			if minutes == 1 {
				return "1 minute"
			}
			return strconv.FormatInt(minutes, 10) + " minutes"
		}
		if seconds > 60 {
			return strconv.FormatInt(seconds/60, 10) + " min " + strconv.FormatInt(seconds%60, 10) + " sec"
		}
		return strconv.FormatInt(seconds, 10) + " seconds"
	}
	return value
}
