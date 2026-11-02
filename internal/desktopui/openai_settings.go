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
)

var openAISecrets = mygo.Secrets.Service(openAIService)

func openAIGenerator(modelName string, keyOverride ...string) model.Generator {
	config := model.FromEnvironment()
	if strings.TrimSpace(modelName) != "" {
		config.Model = strings.TrimSpace(modelName)
	}
	return openAIGeneratorConfig(config, keyOverride...)
}

func openAIGeneratorWithPreferences(prefs preferences, keyOverride ...string) model.Generator {
	config := model.Config{
		Model:           strings.TrimSpace(prefs.Model),
		APIKey:          strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		Timeout:         secondsPreference(prefs.TimeoutSeconds, 90),
		ReasoningEffort: prefs.ReasoningEffort,
		TextVerbosity:   prefs.TextVerbosity,
		MaxOutputTokens: integerPreference(prefs.MaxOutputTokens, model.DefaultMaxOutputTokens),
		MaxRetries:      int(integerPreference(prefs.MaxRetries, int64(model.DefaultMaxRetries))),
	}
	return openAIGeneratorConfig(config, keyOverride...)
}

func openAIGeneratorConfig(config model.Config, keyOverride ...string) model.Generator {
	if key, err := openAISecrets.Get(openAIKeyName); err == nil && strings.TrimSpace(string(key)) != "" {
		config.APIKey = strings.TrimSpace(string(key))
	}
	if len(keyOverride) > 0 && strings.TrimSpace(keyOverride[0]) != "" {
		config.APIKey = strings.TrimSpace(keyOverride[0])
	}
	return model.Generator{Config: config}
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
	a.settingsTab = 0
	a.checkCredential()
}

func (a *app) checkCredential() {
	if a.credentialChecked || a.credentialLoading {
		return
	}
	a.credentialLoading = true
	win := a.win
	go func() {
		_, err := openAISecrets.Get(openAIKeyName)
		if win == nil {
			return
		}
		win.Update(func() {
			a.credentialLoading = false
			a.credentialChecked = true
			a.credentialSaved = err == nil
			a.credentialError = ""
			if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
				a.credentialError = err.Error()
			}
		})
	}()
}

func (a *app) saveSettings() {
	if a.settingsSaving {
		return
	}
	a.prefs.Model = strings.TrimSpace(a.prefs.Model)
	if err := savePreferences(a.dataDir, a.prefs); err != nil {
		a.adt.Error("Couldn’t save settings", err.Error())
		return
	}
	a.publishRoutinePreferences()
	key := strings.TrimSpace(a.apiKeyDraft)
	if key == "" {
		description := "Your preferences are ready to use."
		if a.credentialSaved {
			description = "Your OpenAI API key remains saved securely."
		} else if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
			description = "OpenAI will use the key provided by your environment."
		} else {
			description = "Add an API key when you're ready to use model tasks."
		}
		a.adt.Success("Settings saved", description)
		return
	}
	a.settingsSaving = true
	win := a.win
	go func() {
		err := openAISecrets.Set(openAIKeyName, []byte(key))
		if win == nil {
			return
		}
		win.Update(func() {
			a.settingsSaving = false
			if err != nil {
				a.adt.Error("API key wasn't saved", err.Error())
				return
			}
			a.apiKeyDraft = ""
			a.credentialChecked = true
			a.credentialSaved = true
			a.credentialError = ""
			a.adt.Success("Settings saved", "Your OpenAI API key is saved securely.")
		})
	}()
}

func (a *app) removeAPIKey() {
	if a.settingsSaving || a.credentialLoading {
		return
	}
	a.settingsSaving = true
	win := a.win
	go func() {
		err := openAISecrets.Delete(openAIKeyName)
		if errors.Is(err, mygo.ErrSecretNotFound) {
			err = nil
		}
		if win == nil {
			return
		}
		win.Update(func() {
			a.settingsSaving = false
			if err != nil {
				a.adt.Error("Couldn't remove the API key", err.Error())
				return
			}
			a.apiKeyDraft = ""
			a.credentialChecked = true
			a.credentialSaved = false
			a.credentialError = ""
			a.modelConnection = "Not tested"
			a.adt.Success("API key removed", "The saved key was removed from macOS Keychain.")
		})
	}()
}

func (a *app) credentialDescription() string {
	if a.credentialLoading {
		return "Checking macOS Keychain…"
	}
	if a.credentialSaved {
		return "API key saved in macOS Keychain"
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return "Using OPENAI_API_KEY from the environment"
	}
	if a.credentialError != "" {
		return "System credential store unavailable: " + a.credentialError
	}
	return "No API key saved yet"
}

func (a *app) testModel() {
	if a.cancelRun != nil {
		a.adt.Warning("A Dot is already running", "Wait for the current reply to finish before testing the connection.")
		return
	}
	a.modelConnection = "Checking…"
	a.adt.Info("Checking connection", "AETHER is testing the selected OpenAI model.")
	prefs := a.prefs
	keyDraft := a.apiKeyDraft
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := openAIGeneratorWithPreferences(prefs, keyDraft).Test(ctx)
		if win != nil {
			win.Update(func() {
				if err != nil {
					a.modelConnection = "Connection failed"
					a.adt.Error("Connection failed", err.Error())
				} else {
					a.modelConnection = "Connected"
					a.adt.Success("Connection verified", "The selected model is ready for your Dots.")
				}
			})
		}
	}()
}
