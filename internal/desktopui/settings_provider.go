package desktopui

import (
	"os"
	"strings"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func (a *app) providerSettings(c *ui.Context, theme *ui.Theme) {
	if providerFromSelection(a.providerSelection) != a.prefs.Provider {
		a.providerSelection = providerSelectionLabel(a.prefs.Provider)
	}
	providerName := providerDisplayName(a.prefs.Provider)
	modelName := a.prefs.selectedModel()
	card := ui.Column(c).FillWidth().Gap(15).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Box(c).Size(34, 34).Shrink(0).Radius(11).
				Background(theme.Accent.Alpha(0.14)).TextColor(theme.Accent).Children(func() {
				ui.Icon(c, iconSparkles).Size(18, 18)
			})
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, providerName).FontSize(15).Bold()
				ui.Text(c, "Choose a provider for replies and scheduled runs.").FontSize(12).TextColor(theme.TextMuted)
			})
			ui.Spacer(c)
			state, color := connectionBadge(theme, a.modelConnection)
			ui.Badge(c, state).TextColor(color)
		})
		ui.Divider(c)
		ui.Field(c, "Provider", func() {
			selector := ui.Select(c.Key("settings-provider-select"), &a.providerSelection, []string{"OpenAI", "OpenRouter", "Custom OpenAI-compatible"}).
				Label("AI provider").FillWidth().Disabled(a.settingsSaving || a.modelConnection == "Checking…")
			if selector.Changed() {
				a.switchProvider(providerFromSelection(a.providerSelection))
			}
		})
		if a.prefs.Provider == string(model.ProviderCompatible) {
			ui.Field(c, "Endpoint URL", func() {
				field := ui.TextInput(c.Key("settings-compatible-endpoint"), &a.prefs.CompatibleBaseURL).
					Placeholder("https://api.example.com/v1 or http://localhost:11434/v1").
					FillWidth().Label("OpenAI-compatible API base URL").
					ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
				if field.Changed() {
					a.modelConnection = "Not tested"
				}
			})
		}
		ui.Field(c, "Model", func() {
			field := ui.TextInput(c.Key("settings-provider-model-"+a.prefs.Provider), modelName).
				Placeholder(providerModelPlaceholder(a.prefs.Provider)).FillWidth().Label(providerName + " model").
				ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
			if field.Changed() {
				a.modelConnection = "Not tested"
			}
		})
		ui.Field(c, "API key", func() {
			field := ui.TextInput(c.Key("settings-provider-key-"+a.prefs.Provider), &a.apiKeyDraft).
				Placeholder(apiKeyPlaceholder(a.credentialSaved)).FillWidth().Label(providerKeyLabel(a.prefs.Provider)).
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
		ui.Column(c).FillWidth().Gap(5).Padding(12, 14).Radius(11).
			Background(theme.Background).Border(1, theme.Border).Children(func() {
			ui.Text(c, "Your data and privacy").FontSize(11).Bold()
			ui.Text(c, providerPrivacyDescription(a.prefs.Provider)).
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			ui.Text(c, "API keys are stored in the system credential store and are never written to preferences, Dot data, or logs.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		ui.Row(c).Gap(9).Children(func() {
			test := ui.Button(c.Key("settings-test-provider"), "").Grow(1).Padding(10, 12).Gap(8).Radius(10).
				Label("Test connection").Disabled(a.settingsSaving || a.modelConnection == "Checking…" || strings.TrimSpace(*modelName) == "")
			test.Children(func() {
				ui.Icon(c, iconPlay).Size(15, 15).TextColor(theme.Accent)
				ui.Text(c, "Test connection").FontSize(12)
			})
			if test.Clicked() {
				a.testModel()
			}
			if a.credentialSaved {
				remove := ui.Button(c.Key("settings-remove-provider-key"), "Remove saved key").Padding(10, 12).
					Label("Remove saved key").Disabled(a.settingsSaving || a.credentialLoading || a.modelConnection == "Checking…")
				if remove.Clicked() {
					a.removeAPIKey()
				}
			}
		})
	})
}

func providerFromSelection(selection string) string {
	switch selection {
	case "OpenRouter":
		return string(model.ProviderOpenRouter)
	case "Custom OpenAI-compatible":
		return string(model.ProviderCompatible)
	default:
		return string(model.ProviderOpenAI)
	}
}

func providerSelectionLabel(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "OpenRouter"
	case model.ProviderCompatible:
		return "Custom OpenAI-compatible"
	default:
		return "OpenAI"
	}
}

func providerModelPlaceholder(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "openai/gpt-6.1-sol"
	case model.ProviderCompatible:
		return "Model name supported by your endpoint"
	default:
		return model.DefaultModel
	}
}

func providerKeyLabel(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "OpenRouter API key"
	case model.ProviderCompatible:
		return "API key (optional for a local endpoint)"
	default:
		return "OpenAI API key"
	}
}

func providerPrivacyDescription(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "AETHER sends your message, Dot profile, recent conversation context, and any named skill to OpenRouter, which routes the request to the selected model provider. The model can request only AETHER's local time and calculator functions."
	case model.ProviderCompatible:
		return "AETHER sends your message, Dot profile, recent conversation context, and any named skill to the endpoint you configure. The model can request only AETHER's local time and calculator functions."
	default:
		return "AETHER sends your message, Dot profile, recent conversation context, and any named skill to OpenAI. Dots can't access files, folders, or other apps. AETHER asks the Responses API not to store a separate copy of each reply."
	}
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
	if a.credentialSaved || strings.TrimSpace(a.apiKeyDraft) != "" || strings.TrimSpace(os.Getenv(providerEnvironmentKey(model.ProviderID(a.prefs.Provider)))) != "" ||
		(a.prefs.Provider == string(model.ProviderCompatible) && isLoopbackEndpoint(a.prefs.CompatibleBaseURL)) {
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
	return "Enter API key"
}
