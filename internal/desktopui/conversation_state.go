package desktopui

import (
	"time"

	"aether-dots/internal/task"
)

type conversationStatusSummary struct {
	status    task.Status
	updatedAt time.Time
}

func findConversation(db task.Database, id string) (task.Conversation, bool) {
	for _, conversation := range db.Conversations {
		if conversation.ID == id {
			return conversation, true
		}
	}
	return task.Conversation{}, false
}

func validProfileDelegateIDs(db task.Database, sourceID, spaceID string, selected []string) []string {
	allowed := make(map[string]bool, len(selected))
	for _, id := range selected {
		allowed[id] = true
	}
	valid := make([]string, 0, min(len(selected), task.MaxDelegatedDots))
	for _, conversation := range db.Conversations {
		if conversation.ID == sourceID || conversation.SpaceID != spaceID || !allowed[conversation.ID] {
			continue
		}
		valid = append(valid, conversation.ID)
		if len(valid) == task.MaxDelegatedDots {
			break
		}
	}
	return valid
}

func conversationDots(db task.Database, conversationID string) []task.Dot {
	dots := make([]task.Dot, 0)
	for _, dot := range db.Dots {
		if dot.ConversationID == conversationID {
			dots = append(dots, dot)
		}
	}
	return dots
}

// latestConversationDots finds the newest task per conversation in one pass.
// The sidebar and header reuse the result instead of filtering and sorting all
// tasks once for every visible conversation.
func latestConversationDots(db task.Database) map[string]conversationStatusSummary {
	latest := make(map[string]conversationStatusSummary, min(len(db.Dots), len(db.Conversations)))
	for _, dot := range db.Dots {
		if dot.ConversationID == "" {
			continue
		}
		current, ok := latest[dot.ConversationID]
		if !ok || dot.UpdatedAt.After(current.updatedAt) {
			latest[dot.ConversationID] = conversationStatusSummary{status: dot.Status, updatedAt: dot.UpdatedAt}
		}
	}
	return latest
}

func findConversationDot(db task.Database, conversationID, dotID string) (task.Dot, bool) {
	dot, ok := findDot(db, dotID)
	return dot, ok && dot.ConversationID == conversationID
}
