package desktopui

import (
	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
)

func generatorForConversationRun(prefs preferences, db *store.Store, dotID string, githubService *github.Service, onProgress func()) model.Provider {
	providerForDot := func(id string) model.Provider {
		provider := generatorForPreferences(prefs)
		if dot, ok := db.GetDot(id); ok && dot.ConversationID != "" {
			provider = model.WithPromptCacheKey(provider, "aether-"+dot.ConversationID)
		}
		return engine.WithGitHubTools(db, id, provider, githubService)
	}
	provider := providerForDot(dotID)
	return engine.WithSpecialistDelegationFactory(db, dotID, provider, providerForDot, onProgress)
}
