package desktopui

import (
	"context"

	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/computeruse"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
)

func generatorForConversationRun(prefs preferences, db *store.Store, dotID string, githubService *github.Service, onProgress func()) model.Provider {
	return buildConversationRunGenerator(context.Background(), prefs, db, dotID, githubService, nil, onProgress, false)
}

func generatorForInteractiveConversationRun(ctx context.Context, prefs preferences, db *store.Store, dotID string, githubService *github.Service, computerUseService *computeruse.Service, onProgress func()) model.Provider {
	return buildConversationRunGenerator(ctx, prefs, db, dotID, githubService, computerUseService, onProgress, true)
}

func generatorForConversationRunWithComputerUse(prefs preferences, db *store.Store, dotID string, githubService *github.Service, computerUseService *computeruse.Service, onProgress func()) model.Provider {
	return buildConversationRunGenerator(context.Background(), prefs, db, dotID, githubService, computerUseService, onProgress, false)
}

func buildConversationRunGenerator(ctx context.Context, prefs preferences, db *store.Store, dotID string, githubService *github.Service, computerUseService *computeruse.Service, onProgress func(), includeMCP bool) model.Provider {
	providerForDot := func(id string) model.Provider {
		provider := generatorForDotPreferences(prefs, db, id)
		if dot, ok := db.GetDot(id); ok && dot.ConversationID != "" {
			provider = model.WithPromptCacheKey(provider, "aether-"+dot.ConversationID)
		}
		provider = engine.WithGitHubTools(db, id, provider, githubService)
		provider = engine.WithComputerUseTools(db, id, provider, computerUseService)
		if includeMCP {
			provider = engine.WithMCPTools(ctx, db, id, provider)
		}
		return provider
	}
	provider := providerForDot(dotID)
	if includeMCP {
		// Proposal-only GitHub write tools exist for interactive desktop runs
		// only; Routines, iPhone messages and delegated Dots never get them.
		provider = engine.WithGitHubWriteTools(db, dotID, provider, githubService)
	}
	return engine.WithSpecialistDelegationFactory(db, dotID, provider, providerForDot, onProgress)
}
