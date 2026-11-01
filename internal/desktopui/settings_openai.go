package desktopui

import (
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) openAISettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(15).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Box(c).Size(34, 34).Shrink(0).Radius(11).
				Background(theme.Accent.Alpha(0.14)).TextColor(theme.Accent).Children(func() {
				ui.Icon(c, iconSparkles).Size(18, 18)
			})
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "OpenAI").FontSize(15).Bold()
				ui.Text(c, "Official OpenAI Go SDK · Responses API").FontSize(12).TextColor(theme.TextMuted)
			})
			ui.Spacer(c)
			state, color := connectionBadge(theme, a.modelConnection)
			ui.Badge(c, state).TextColor(color)
		})
		ui.Divider(c)
		ui.Field(c, "Model", func() {
			field := ui.TextInput(c.Key("settings-openai-model"), &a.prefs.Model).
				Placeholder("gpt-6.1-sol").FillWidth().Label("OpenAI model").ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
			if field.Changed() {
				a.modelConnection = "Not tested"
			}
		})
		ui.Field(c, "API key", func() {
			field := ui.TextInput(c.Key("settings-openai-key"), &a.apiKeyDraft).
				Placeholder(apiKeyPlaceholder(a.credentialSaved)).FillWidth().Label("OpenAI API key").
				Password().ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
			if field.Changed() {
				a.modelConnection = "Not tested"
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			statusText := a.credentialDescription()
			if strings.TrimSpace(a.apiKeyDraft) != "" {
				statusText = "A new key is ready to test and save securely"
			}
			ui.Box(c).Size(7, 7).Shrink(0).Radius(4).Background(credentialColor(theme, a))
			ui.Text(c, statusText).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		ui.Text(c, "AETHER sends your messages, Dot profile, and recent chat context to OpenAI to generate replies. Dots cannot access files or folders. Saved keys stay in the system credential store; response storage is disabled.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		ui.Row(c).Gap(9).Children(func() {
			test := ui.Button(c.Key("settings-test-openai"), "").Grow(1).Padding(10, 12).Gap(8).Radius(10).
				Label("Test connection").Disabled(a.settingsSaving || a.modelConnection == "Checking…" || strings.TrimSpace(a.prefs.Model) == "")
			test.Children(func() {
				ui.Icon(c, iconPlay).Size(15, 15).TextColor(theme.Accent)
				ui.Text(c, "Test connection").FontSize(12)
			})
			if test.Clicked() {
				a.testModel()
			}
			if a.credentialSaved {
				remove := ui.Button(c.Key("settings-remove-openai-key"), "Remove saved key").Padding(10, 12).
					Label("Remove saved key").Disabled(a.settingsSaving || a.credentialLoading || a.modelConnection == "Checking…")
				if remove.Clicked() {
					a.removeAPIKey()
				}
			}
		})
	})
}

func connectionBadge(theme *ui.Theme, state string) (string, ui.Color) {
	switch state {
	case "Connected":
		return "Connected", theme.Success
	case "Checking…":
		return "Checking", theme.Accent
	case "Connection failed":
		return "Connection failed", theme.Danger
	default:
		return "Not tested", theme.TextMuted
	}
}

func credentialColor(theme *ui.Theme, a *app) ui.Color {
	if a.credentialLoading || a.settingsSaving {
		return theme.Accent
	}
	if a.credentialSaved || strings.TrimSpace(a.apiKeyDraft) != "" || strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return theme.Success
	}
	if a.credentialError != "" {
		return theme.Warning
	}
	return theme.Warning
}

func apiKeyPlaceholder(saved bool) string {
	if saved {
		return "Saved securely · enter a new key to replace"
	}
	return "sk-…"
}
