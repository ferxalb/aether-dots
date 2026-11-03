package desktopui

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var modelSecrets = mygo.Secrets.Service(openAIService)

func generatorForPreferences(prefs preferences, keyOverride ...string) model.Provider {
	provider := model.ProviderID(prefs.Provider)
	key := providerAPIKey(provider)
	if len(keyOverride) > 0 && strings.TrimSpace(keyOverride[0]) != "" {
		key = strings.TrimSpace(keyOverride[0])
	}
	config := model.Config{
		Provider:               provider,
		Model:                  strings.TrimSpace(*prefs.selectedModel()),
		BaseURL:                strings.TrimSpace(prefs.CompatibleBaseURL),
		APIKey:                 key,
		Timeout:                secondsPreference(prefs.TimeoutSeconds, 90),
		ReasoningEffort:        prefs.ReasoningEffort,
		TextVerbosity:          prefs.TextVerbosity,
		MaxOutputTokens:        integerPreference(prefs.MaxOutputTokens, 0),
		MaxRetries:             int(integerPreference(prefs.MaxRetries, int64(model.DefaultMaxRetries))),
		EnableAnthropicThreads: prefs.AnthropicMessageThreads,
		DisableNativeTools: provider == model.ProviderOpenRouter &&
			prefs.OpenRouterCatalogModel == strings.TrimSpace(prefs.OpenRouterModel) &&
			prefs.OpenRouterCatalogModel != "" && !prefs.OpenRouterModelSupportsTools,
		DisableReasoningEffort: openRouterReasoningEffortDisabled(prefs),
	}
	return model.NewProvider(config)
}

func openRouterReasoningEffortDisabled(prefs preferences) bool {
	if prefs.Provider != string(model.ProviderOpenRouter) ||
		prefs.OpenRouterCatalogModel == "" ||
		prefs.OpenRouterCatalogModel != strings.TrimSpace(prefs.OpenRouterModel) ||
		!prefs.OpenRouterModelSupportsReasoningSet {
		return false
	}
	if !prefs.OpenRouterModelSupportsReasoning {
		return true
	}
	effort := strings.TrimSpace(prefs.ReasoningEffort)
	if effort == "" || prefs.OpenRouterModelSupportedEfforts == "" {
		return false
	}
	for _, supported := range strings.Split(prefs.OpenRouterModelSupportedEfforts, ",") {
		if strings.EqualFold(effort, supported) {
			return false
		}
	}
	return true
}

func providerAPIKey(provider model.ProviderID) string {
	if keyName := providerKeyName(provider); keyName != "" {
		if key, err := modelSecrets.Get(keyName); err == nil {
			value := strings.TrimSpace(string(key))
			clear(key)
			if value != "" {
				return value
			}
		}
	}
	return strings.TrimSpace(os.Getenv(providerEnvironmentKey(provider)))
}

func providerKeyName(provider model.ProviderID) string {
	switch provider {
	case model.ProviderOpenAI:
		return openAIKeyName
	case model.ProviderAnthropic:
		return anthropicKeyName
	case model.ProviderXAI:
		return xAIKeyName
	case model.ProviderOpenRouter:
		return openRouterKeyName
	case model.ProviderCompatible:
		return compatibleKeyName
	default:
		return ""
	}
}

func providerEnvironmentKey(provider model.ProviderID) string {
	switch provider {
	case model.ProviderOpenAI:
		return "OPENAI_API_KEY"
	case model.ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case model.ProviderXAI:
		return "XAI_API_KEY"
	case model.ProviderOpenRouter:
		return "OPENROUTER_API_KEY"
	case model.ProviderCompatible:
		return "AETHER_COMPATIBLE_API_KEY"
	default:
		return ""
	}
}

func providerDisplayName(provider string) string {
	switch model.ProviderID(provider) {
	case model.ProviderOpenAI:
		return "OpenAI"
	case model.ProviderAnthropic:
		return "Anthropic Claude"
	case model.ProviderXAI:
		return "xAI Grok"
	case model.ProviderOpenRouter:
		return "OpenRouter"
	case model.ProviderCompatible:
		return "Custom OpenAI-compatible endpoint"
	default:
		return "Model provider"
	}
}

func (a *app) switchProvider(provider string) {
	if !validProvider(provider) || provider == a.prefs.Provider {
		return
	}
	if a.apiKeyDrafts == nil {
		a.apiKeyDrafts = make(map[string]string)
	}
	a.apiKeyDrafts[a.prefs.Provider] = a.apiKeyDraft
	a.prefs.Provider = provider
	if provider == string(model.ProviderOpenAI) && strings.TrimSpace(a.prefs.ReasoningEffort) == "" {
		a.prefs.ReasoningEffort = model.DefaultReasoningEffort
	}
	if provider == string(model.ProviderXAI) && !validProviderReasoningEffort(provider, a.prefs.ReasoningEffort) {
		a.prefs.ReasoningEffort = "high"
	}
	if provider == string(model.ProviderAnthropic) && !validProviderReasoningEffort(provider, a.prefs.ReasoningEffort) {
		a.prefs.ReasoningEffort = "high"
	}
	a.providerSelection = providerSelectionLabel(provider)
	a.apiKeyDraft = a.apiKeyDrafts[provider]
	a.credentialProvider = ""
	a.credentialChecked = false
	a.credentialLoading = false
	a.credentialSaved = false
	a.credentialError = ""
	a.modelConnection = "Not tested"
	a.checkCredential()
}

func integerPreference(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func secondsPreference(value string, fallback int64) time.Duration {
	return time.Duration(integerPreference(value, fallback)) * time.Second
}

func (a *app) openSettings() {
	a.settingsOpen = true
	a.activityOpen = false
	a.settingsTab = 0
	a.providerSelection = providerSelectionLabel(a.prefs.Provider)
	a.checkCredential()
}

func (a *app) checkCredential() {
	provider := a.prefs.Provider
	if a.credentialProvider == provider && (a.credentialChecked || a.credentialLoading) {
		return
	}
	a.credentialProvider = provider
	a.credentialLoading = true
	win := a.win
	if win == nil {
		a.credentialLoading = false
		a.credentialChecked = true
		return
	}
	go func() {
		key, err := modelSecrets.Get(providerKeyName(model.ProviderID(provider)))
		saved := err == nil && strings.TrimSpace(string(key)) != ""
		clear(key)
		credentialError := ""
		if errors.Is(err, errors.ErrUnsupported) {
			credentialError = "system credential store is not supported on this system"
		} else if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
			credentialError = err.Error()
		}
		win.Update(func() {
			if a.prefs.Provider != provider {
				return
			}
			a.credentialLoading = false
			a.credentialChecked = true
			a.credentialSaved = saved
			a.credentialError = credentialError
		})
	}()
}

func (a *app) saveSettings() {
	if a.settingsSaving {
		return
	}
	if selected := a.prefs.selectedModel(); selected != nil {
		*selected = strings.TrimSpace(*selected)
	}
	if err := savePreferences(a.dataDir, a.prefs); err != nil {
		a.adt.Error("Couldn’t save settings", err.Error())
		return
	}
	if !a.prefs.AnthropicMessageThreads {
		model.ClearAnthropicThreads()
	}
	a.publishRoutinePreferences()
	key := strings.TrimSpace(a.apiKeyDraft)
	if key == "" {
		description := "Your preferences are ready to use."
		switch {
		case a.credentialLoading:
			description = "Your preferences are ready. Checking for a saved credential."
		case a.credentialSaved:
			description = "Your " + providerDisplayName(a.prefs.Provider) + " credential remains saved securely."
		case strings.TrimSpace(os.Getenv(providerEnvironmentKey(model.ProviderID(a.prefs.Provider)))) != "":
			description = "The provider will use its API key from your environment."
		case a.prefs.Provider == string(model.ProviderCompatible) && isLoopbackEndpoint(a.prefs.CompatibleBaseURL):
			description = "Your local endpoint is configured and does not require an API key."
		default:
			description = "Add an API key when you're ready to use this provider."
		}
		notice := ADTNotice{Title: "Settings saved", Description: description, Level: ADTSuccess}
		if !a.credentialLoading && !a.credentialSaved && strings.TrimSpace(os.Getenv(providerEnvironmentKey(model.ProviderID(a.prefs.Provider)))) == "" && a.prefs.Provider != string(model.ProviderCompatible) {
			notice.Action = "Add API key"
			notice.OnAction = func() {
				a.settingsOpen = true
				a.settingsTab = 1
				a.settingsScroll[1] = ui.ScrollState{}
			}
		}
		a.adt.Push(notice)
		return
	}
	a.settingsSaving = true
	const toastID = "settings-provider-key-save"
	a.adt.Loading(toastID, "Saving API key", "Keeping it in the system credential store.")
	win := a.win
	provider := a.prefs.Provider
	keyName := providerKeyName(model.ProviderID(provider))
	go func() {
		secret := []byte(key)
		err := modelSecrets.Set(keyName, secret)
		clear(secret)
		if win == nil {
			return
		}
		win.Update(func() {
			a.settingsSaving = false
			if err != nil {
				a.adt.Push(ADTNotice{ID: toastID, Title: "API key wasn't saved", Description: err.Error(), Level: ADTError})
				return
			}
			if a.apiKeyDrafts == nil {
				a.apiKeyDrafts = make(map[string]string)
			}
			a.apiKeyDraft = ""
			a.apiKeyDrafts[provider] = ""
			a.credentialProvider = provider
			a.credentialChecked = true
			a.credentialSaved = true
			a.credentialError = ""
			a.adt.Push(ADTNotice{ID: toastID, Title: "API key saved", Description: "Your key is stored in the system credential store.", Level: ADTSuccess})
		})
	}()
}

func (a *app) removeAPIKey() {
	if a.settingsSaving || a.credentialLoading {
		return
	}
	a.settingsSaving = true
	const toastID = "settings-provider-key-remove"
	a.adt.Loading(toastID, "Removing API key", "Updating your secure credential store.")
	win := a.win
	provider := a.prefs.Provider
	keyName := providerKeyName(model.ProviderID(provider))
	go func() {
		err := modelSecrets.Delete(keyName)
		if errors.Is(err, mygo.ErrSecretNotFound) {
			err = nil
		}
		if win == nil {
			return
		}
		win.Update(func() {
			a.settingsSaving = false
			if err != nil {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Couldn't remove the API key", Description: err.Error(), Level: ADTError})
				return
			}
			if a.apiKeyDrafts == nil {
				a.apiKeyDrafts = make(map[string]string)
			}
			a.apiKeyDraft = ""
			a.apiKeyDrafts[provider] = ""
			a.credentialProvider = provider
			a.credentialChecked = true
			a.credentialSaved = false
			a.credentialError = ""
			a.modelConnection = "Not tested"
			a.adt.Push(ADTNotice{ID: toastID, Title: "API key removed", Description: "The saved key was removed from the system credential store.", Level: ADTSuccess})
		})
	}()
}

func (a *app) credentialDescription() string {
	if a.credentialLoading {
		return "Checking the system credential store…"
	}
	if a.credentialSaved {
		return "API key saved securely in the system credential store"
	}
	if envName := providerEnvironmentKey(model.ProviderID(a.prefs.Provider)); strings.TrimSpace(os.Getenv(envName)) != "" {
		return "Using " + envName + " from the environment"
	}
	if a.prefs.Provider == string(model.ProviderCompatible) && isLoopbackEndpoint(a.prefs.CompatibleBaseURL) {
		return "API key is optional for a local endpoint"
	}
	if a.credentialError != "" {
		return "System credential store unavailable: " + a.credentialError
	}
	return "No API key saved yet"
}

func (a *app) testModel() {
	if a.hasActiveRuns() {
		a.adt.Warning("A Dot is already running", "Wait for the current reply to finish before testing the connection.")
		return
	}
	if selected := a.prefs.selectedModel(); strings.TrimSpace(*selected) == "" {
		a.adt.Error("Model name is required", "Enter a model name before testing the connection.")
		return
	}
	a.modelConnection = "Checking…"
	const toastID = "settings-provider-connection"
	provider := a.prefs.Provider
	a.adt.Loading(toastID, "Checking connection", "Testing the selected "+providerDisplayName(provider)+" model.")
	prefs := a.prefs
	keyDraft := a.apiKeyDraft
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := generatorForPreferences(prefs, keyDraft).Test(ctx)
		if win != nil {
			win.Update(func() {
				if a.prefs.Provider != provider {
					return
				}
				if err != nil {
					a.modelConnection = "Connection failed"
					a.adt.Push(ADTNotice{ID: toastID, Title: "Connection failed", Description: err.Error(), Level: ADTError})
				} else {
					a.modelConnection = "Connected"
					a.adt.Push(ADTNotice{ID: toastID, Title: "Connection verified", Description: providerDisplayName(provider) + " is ready for your Dots.", Level: ADTSuccess})
				}
			})
		}
	}()
}

func isLoopbackEndpoint(raw string) bool {
	endpoint, err := model.ValidateBaseURL(raw)
	if err != nil {
		return false
	}
	return strings.HasPrefix(endpoint, "http://localhost") || strings.HasPrefix(endpoint, "http://127.") || strings.HasPrefix(endpoint, "http://[::1]")
}
