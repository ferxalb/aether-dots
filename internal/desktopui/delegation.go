package desktopui

import (
	"aether-dots/internal/engine"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
)

func generatorForConversationRun(prefs preferences, db *store.Store, dotID string, onProgress func()) model.Provider {
	provider := generatorForPreferences(prefs)
	return engine.WithSpecialistDelegation(db, dotID, provider, onProgress)
}
