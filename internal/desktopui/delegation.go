package desktopui

import (
	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
)

func generatorForConversationRun(prefs preferences, db *store.Store, dotID string, githubService *github.Service, onProgress func()) model.Provider {
	providerForDot := func(id string) model.Provider {
		return engine.WithGitHubTools(db, id, generatorForPreferences(prefs), githubService)
	}
	provider := providerForDot(dotID)
	return engine.WithSpecialistDelegationFactory(db, dotID, provider, providerForDot, onProgress)
}
