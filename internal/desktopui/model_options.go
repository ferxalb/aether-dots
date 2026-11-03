package desktopui

import (
	"strconv"
	"strings"

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
		switch model.ProviderID(a.prefs.Provider) {
		case model.ProviderOpenAI:
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort,
					[]string{"none", "minimal", "low", "medium", "high", "xhigh", "max"})
				modelChoice(c, theme, "settings-verbosity", "Reply detail", &a.prefs.TextVerbosity,
					[]string{"low", "medium", "high"})
			})
		case model.ProviderXAI:
			modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort,
				[]string{"low", "medium", "high", "xhigh"})
			ui.Text(c, "Grok uses xAI's Responses API. AETHER keeps provider web and X search tools disabled.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		case model.ProviderAnthropic:
			modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort,
				[]string{"", "low", "medium", "high", "xhigh", "max"})
			ui.Text(c, "AETHER enables adaptive thinking and applies this effort level on supported Claude models. Internal thinking stays out of the chat. Only explicitly listed AETHER client functions are available.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		case model.ProviderOpenRouter:
			metadata, known := a.selectedOpenRouterModel()
			if known && metadata.ReasoningMetadataKnown && !metadata.SupportsReasoning {
				ui.Text(c, "This OpenRouter model does not declare reasoning-effort support. AETHER omits that parameter and lets the model use its default.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			} else {
				options := reasoningEffortOptions(metadata, known)
				modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort, options)
				if known && !reasoningEffortSupported(metadata, a.prefs.ReasoningEffort) {
					ui.Text(c, "The saved effort is not listed for this model. AETHER uses the model default until you choose a supported level.").
						FontSize(11).TextColor(theme.TextMuted).FillWidth()
				} else if !known || !metadata.ReasoningMetadataKnown {
					ui.Text(c, "Effort support for this model ID is unknown. OpenRouter may ignore or reject the optional reasoning_effort parameter; choose Use model default to omit it.").
						FontSize(11).TextColor(theme.TextMuted).FillWidth()
				}
			}
			ui.Text(c, "Reply detail is controlled by the selected model; Chat Completions does not receive OpenAI Responses verbosity settings.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		case model.ProviderCompatible:
			modelChoice(c, theme, "settings-reasoning", "Thinking effort", &a.prefs.ReasoningEffort,
				reasoningEffortOptions(model.OpenRouterModel{}, false))
			ui.Text(c, "AETHER sends this as the optional Chat Completions reasoning_effort parameter. Use model default to omit it if your endpoint or model does not support it.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			modelChoice(c, theme, "settings-output-tokens", "Reply token limit (optional)", &a.prefs.MaxOutputTokens,
				[]string{"", "256", "512", "1024", "2048", "4096", "8192", "16384", "32768", "65536"})
			modelChoice(c, theme, "settings-retries", "Retries for temporary errors", &a.prefs.MaxRetries,
				[]string{"0", "1", "2", "3", "4", "5"})
		})
		modelChoice(c, theme, "settings-timeout", "Stop waiting after", &a.prefs.TimeoutSeconds,
			[]string{"30", "45", "60", "90", "120", "180", "300"})
		if model.ProviderID(a.prefs.Provider) == model.ProviderAnthropic {
			ui.Text(c, "Anthropic requires an output token cap; when this is blank AETHER uses 16,384. Claude reasoning tokens count toward this cap. Other providers omit an unset cap. The model’s context window is not set here.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		} else {
			ui.Text(c, "With no reply token limit, AETHER omits that parameter and lets the selected model use its own default. The model’s context window is provided by the model and is not set here.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
	})
}

func modelChoice(c *ui.Context, theme *ui.Theme, key, label string, value *string, options []string) {
	field := ui.Column(c).Grow(1).FillWidth().MinWidth(0).Gap(5)
	field.Children(func() {
		ui.Text(c, label).FontSize(11).Bold()
		dropdownSelect(c, theme, key, label, value,
			stringDropdownOptions(options, func(option string) string { return modelChoiceLabel(key, option) }), false)
	})
}

func reasoningEffortOptions(metadata model.OpenRouterModel, known bool) []string {
	all := []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
	if !known || !metadata.ReasoningMetadataKnown || len(metadata.SupportedReasoningEfforts) == 0 {
		return all
	}
	options := []string{""}
	for _, effort := range all[1:] {
		if reasoningEffortSupported(metadata, effort) {
			options = append(options, effort)
		}
	}
	return options
}

func reasoningEffortSupported(metadata model.OpenRouterModel, effort string) bool {
	if strings.TrimSpace(effort) == "" || len(metadata.SupportedReasoningEfforts) == 0 {
		return true
	}
	for _, supported := range metadata.SupportedReasoningEfforts {
		if strings.EqualFold(supported, effort) {
			return true
		}
	}
	return false
}

func modelChoiceLabel(key, value string) string {
	switch key {
	case "settings-reasoning":
		switch value {
		case "":
			return "Use model default"
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
		if value == "" {
			return "Use model default"
		}
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
