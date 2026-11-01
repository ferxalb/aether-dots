package desktopui

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo"
)

var openAISecrets = mygo.Secrets.Service(openAIService)

func openAIGenerator(modelName string, keyOverride ...string) model.Generator {
	config := model.Config{
		Model:   strings.TrimSpace(modelName),
		APIKey:  strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		Timeout: 90 * time.Second,
	}
	if key, err := openAISecrets.Get(openAIKeyName); err == nil && strings.TrimSpace(string(key)) != "" {
		config.APIKey = strings.TrimSpace(string(key))
	}
	if len(keyOverride) > 0 && strings.TrimSpace(keyOverride[0]) != "" {
		config.APIKey = strings.TrimSpace(keyOverride[0])
	}
	return model.Generator{Config: config}
}

func (a *app) openSettings() {
	a.settingsOpen = true
	a.message = ""
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
		a.message = err.Error()
		return
	}
	key := strings.TrimSpace(a.apiKeyDraft)
	if key == "" {
		a.message = "Settings saved."
		if a.credentialSaved {
			a.message = "Settings saved. Your API key remains in the system credential store."
		} else if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
			a.message = "Settings saved. OpenAI will use OPENAI_API_KEY from the environment."
		} else {
			a.message = "Settings saved. Add an API key to use model tasks."
		}
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
				a.message = "Settings saved, but the API key could not be saved to the system credential store: " + err.Error()
				return
			}
			a.apiKeyDraft = ""
			a.credentialChecked = true
			a.credentialSaved = true
			a.credentialError = ""
			a.message = "Settings and OpenAI API key saved securely."
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
				a.message = "Could not remove the saved API key: " + err.Error()
				return
			}
			a.apiKeyDraft = ""
			a.credentialChecked = true
			a.credentialSaved = false
			a.credentialError = ""
			a.modelConnection = "Not tested"
			a.message = "Saved API key removed from the system credential store."
		})
	}()
}

func (a *app) credentialDescription() string {
	if a.credentialLoading {
		return "Checking system credential store…"
	}
	if a.credentialSaved {
		return "API key saved in the system credential store"
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return "Using OPENAI_API_KEY from the environment"
	}
	if a.credentialError != "" {
		return "System credential store unavailable: " + a.credentialError
	}
	return "No API key saved"
}

func (a *app) testModel() {
	if a.cancelRun != nil {
		a.message = "A task is already running."
		return
	}
	a.modelConnection = "Checking…"
	modelName := a.prefs.Model
	keyDraft := a.apiKeyDraft
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := openAIGenerator(modelName, keyDraft).Test(ctx)
		if win != nil {
			win.Update(func() {
				if err != nil {
					a.modelConnection = "Connection failed"
					a.message = "OpenAI connection failed: " + err.Error()
				} else {
					a.modelConnection = "Connected"
					a.message = "OpenAI connection verified with the selected model."
				}
			})
		}
	}()
}
