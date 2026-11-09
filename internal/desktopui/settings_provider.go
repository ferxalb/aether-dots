package desktopui

import (
	"os"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func (a *app) providerSettings(c *ui.Context, theme *ui.Theme) {
	if a.providerSelection == "" || providerFromSelection(a.providerSelection) != a.prefs.Provider {
		a.providerSelection = providerSelectionLabel(a.prefs.Provider)
	}
	providerName := providerDisplayName(a.prefs.Provider)
	modelName := a.prefs.selectedModel()
	busy := a.settingsSaving || a.modelConnection == "Checking…"
	card := ui.Column(c).FillWidth().Radius(16).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		// Identity + connection status. Testing is a quiet secondary action here.
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(18, 20).Children(func() {
			providerLogo(c, a.prefs.Provider, 40)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, providerName).FontSize(17).Bold()
				ui.Text(c, "Choose a provider for replies and scheduled runs.").FontSize(12).TextColor(theme.TextMuted)
			})
			state, color := connectionBadge(theme, a.modelConnection)
			ui.Row(c).AlignItems(ui.Center).Gap(6).Shrink(0).Children(func() {
				ui.Box(c).Size(7, 7).Shrink(0).Radius(4).Background(color)
				label := state
				if when := relativeTime(a.modelConnectionAt, time.Now()); when != "" && (state == "Connected" || state == "Connection failed") {
					label += " · " + when
				}
				ui.Text(c, label).FontSize(12).TextColor(theme.TextMuted)
			})
			test := ui.Button(c.Key("settings-test-provider"), "").Shrink(0).Padding(6, 11).Radius(8).
				Background(ui.Transparent).Border(1, theme.Border).
				Label("Test connection").Disabled(busy || strings.TrimSpace(*modelName) == "")
			if test.Hovered() {
				test.Background(theme.SurfaceHover)
			}
			test.Busy(a.modelConnection == "Checking…")
			testLabel := "Test"
			if a.modelConnection == "Connected" || a.modelConnection == "Connection failed" {
				testLabel = "Retest"
			}
			test.Children(func() { ui.Text(c, testLabel).FontSize(12) })
			if test.Clicked() {
				a.testModel()
			}
		})
		settingsDivider(c, theme)

		settingsRow(c, theme, "Provider", "Who runs your Dots' replies.", func() {
			selector := dropdownSelect(c, theme, "settings-provider-select", "AI provider", &a.providerSelection,
				stringDropdownOptions([]string{"OpenAI", "Anthropic Claude", "xAI Grok", "OpenRouter", "Custom OpenAI-compatible"}, nil), busy)
			if selector.Changed() {
				a.switchProvider(providerFromSelection(a.providerSelection))
			}
		})
		if a.prefs.Provider == string(model.ProviderCompatible) {
			settingsDivider(c, theme)
			settingsRow(c, theme, "Endpoint", "OpenAI-compatible base URL.", func() {
				field := ui.TextInput(c.Key("settings-compatible-endpoint"), &a.prefs.CompatibleBaseURL).
					Placeholder("https://api.example.com/v1 or http://localhost:11434/v1").
					FillWidth().Label("OpenAI-compatible API base URL").ReadOnly(busy).
					Invalid(strings.TrimSpace(a.prefs.CompatibleBaseURL) != "" && !validEndpoint(a.prefs.CompatibleBaseURL))
				if field.Changed() {
					a.syncConnectionState()
				}
			})
		}
		settingsDivider(c, theme)
		if a.prefs.Provider == string(model.ProviderOpenRouter) {
			a.openRouterModelSettings(c, theme, modelName)
		} else {
			settingsRow(c, theme, "Model", "The model used for new replies.", func() {
				field := ui.TextInput(c.Key("settings-provider-model-"+a.prefs.Provider), modelName).
					Placeholder(providerModelPlaceholder(a.prefs.Provider)).FillWidth().Label(providerName + " model").ReadOnly(busy)
				if field.Changed() {
					a.syncConnectionState()
				}
			})
		}
		settingsDivider(c, theme)
		settingsRow(c, theme, "API key", "Stored in your system keychain.", func() {
			field := ui.TextInput(c.Key("settings-provider-key-"+a.prefs.Provider), &a.apiKeyDraft).
				Placeholder(apiKeyPlaceholder(a.credentialSaved)).FillWidth().Label(providerKeyLabel(a.prefs.Provider)).
				Password().ReadOnly(busy)
			if field.Changed() {
				a.invalidateConnection()
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				statusText := a.credentialDescription()
				if strings.TrimSpace(a.apiKeyDraft) != "" {
					statusText = "New key ready · saved when you save changes"
				}
				ui.Box(c).Size(6, 6).Shrink(0).Radius(3).Background(credentialColor(theme, a))
				ui.Text(c, statusText).FontSize(11).TextColor(theme.TextMuted).Grow(1).MinWidth(0)
				if a.credentialSaved {
					remove := ui.Button(c.Key("settings-remove-provider-key"), "").Shrink(0).Padding(3, 7).Radius(6).
						Background(ui.Transparent).Label("Remove saved key").
						Disabled(a.settingsSaving || a.credentialLoading || a.modelConnection == "Checking…")
					if remove.Hovered() {
						remove.Background(theme.Danger.Alpha(0.12))
					}
					remove.Children(func() { ui.Text(c, "Remove").FontSize(11).TextColor(theme.Danger) })
					if remove.Clicked() {
						a.removeAPIKey()
					}
				}
			})
		})
		if a.prefs.Provider == string(model.ProviderAnthropic) {
			settingsDivider(c, theme)
			settingsRow(c, theme, "Message Threads", "Beta · off by default.", func() {
				ui.Checkbox(c.Key("anthropic-message-threads"), &a.prefs.AnthropicMessageThreads,
					"Use short-lived Anthropic Message Threads").FillWidth()
				ui.Text(c, "Later turns send only new messages. Anthropic expires the thread after 5 minutes idle and deletes it within 1 hour; there is no early-delete API and it is not eligible for Zero Data Retention.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
		}
	})
	// Privacy is reference material: quiet footnote, not a framed box.
	ui.Column(c).FillWidth().Gap(4).Padding(2, 6, 0, 6).Children(func() {
		privacyDescription := providerPrivacyDescription(a.prefs.Provider)
		if a.prefs.Provider == string(model.ProviderAnthropic) && a.prefs.AnthropicMessageThreads {
			privacyDescription += " Message Threads are enabled: Anthropic keeps the encrypted thread in memory, expires it after 5 minutes idle, and deletes it no later than 1 hour after last use."
		}
		if a.prefs.Provider == string(model.ProviderOpenRouter) {
			if selected, ok := a.selectedOpenRouterModel(); ok && !selected.SupportsTools {
				privacyDescription += " The selected model does not declare native function support, so AETHER omits the function definitions for it."
			}
		}
		ui.Text(c, "Your data and privacy").FontSize(11).Bold().TextColor(theme.TextMuted)
		ui.Text(c, privacyDescription+" API keys are never written to preferences, Dot data, or logs.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
	})
	if a.prefs.Provider == string(model.ProviderOpenRouter) {
		a.openRouterModelPickerModal(c, theme, modelName)
	}
}

// settingsRow lays out one setting: title and hint on the left, control on the right.
func settingsRow(c *ui.Context, theme *ui.Theme, title, hint string, control func()) {
	ui.Row(c).FillWidth().Gap(24).Padding(16, 20).Children(func() {
		ui.Column(c).Width(170).Shrink(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13).Bold()
			ui.Text(c, hint).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(control)
	})
}

func settingsDivider(c *ui.Context, theme *ui.Theme) {
	ui.Box(c).FillWidth().Height(1).Shrink(0).Background(theme.Border.Alpha(0.6))
}

func validEndpoint(raw string) bool {
	_, err := model.ValidateBaseURL(raw)
	return err == nil
}

func githubToolsKnownUnsupported(prefs preferences) bool {
	return prefs.Provider == string(model.ProviderOpenRouter) &&
		prefs.OpenRouterCatalogModel != "" &&
		prefs.OpenRouterCatalogModel == strings.TrimSpace(prefs.OpenRouterModel) &&
		!prefs.OpenRouterModelSupportsTools
}

func providerFromSelection(selection string) string {
	switch selection {
	case "OpenRouter":
		return string(model.ProviderOpenRouter)
	case "Anthropic Claude":
		return string(model.ProviderAnthropic)
	case "xAI Grok":
		return string(model.ProviderXAI)
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
	case model.ProviderAnthropic:
		return "Anthropic Claude"
	case model.ProviderXAI:
		return "xAI Grok"
	case model.ProviderCompatible:
		return "Custom OpenAI-compatible"
	default:
		return "OpenAI"
	}
}

func profileModelSelection(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "Workspace default"
	}
	return providerSelectionLabel(provider)
}

func profileModelProviderForSelection(selection string) string {
	if selection == "Workspace default" {
		return ""
	}
	return providerFromSelection(selection)
}

func providerModelPlaceholder(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "Select from the live catalog or enter an OpenRouter model ID"
	case model.ProviderAnthropic:
		return model.DefaultAnthropicModel
	case model.ProviderXAI:
		return "grok-4.7"
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
	case model.ProviderAnthropic:
		return "Anthropic API key"
	case model.ProviderXAI:
		return "xAI API key"
	case model.ProviderCompatible:
		return "API key (optional for a local endpoint)"
	default:
		return "OpenAI API key"
	}
}

func providerPrivacyDescription(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenRouter:
		return "AETHER sends your message, Dot profile, saved chat history, and any named skill to OpenRouter, which routes the request to the selected model provider. The picker uses model metadata to omit native functions when a model reports that it does not support them."
	case model.ProviderCompatible:
		return "AETHER sends your message, Dot profile, saved chat history, and any named skill to the endpoint you configure. The model can request only AETHER's local time and calculator functions if your endpoint supports them."
	case model.ProviderAnthropic:
		return "AETHER sends your message, Dot profile, compact chat context, shared Space notes, and any named skill to Anthropic's Messages API. Only explicitly listed AETHER client functions are available; provider-side computer, browser, and web search tools stay off."
	case model.ProviderXAI:
		return "AETHER sends your message, Dot profile, compact chat context, shared Space notes, and any named skill to xAI's Responses API. AETHER keeps response storage disabled and exposes only explicitly listed AETHER client functions; xAI web search and X search stay off."
	default:
		return "AETHER sends your message, Dot profile, saved chat history, and any named skill to OpenAI. Dots can't access files, folders, or other apps. AETHER asks the Responses API not to store a separate copy of each reply."
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
