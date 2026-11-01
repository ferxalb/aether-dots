package desktopui

import "aether-dots/internal/task"

func findConversation(db task.Database, id string) (task.Conversation, bool) {
	for _, conversation := range db.Conversations {
		if conversation.ID == id {
			return conversation, true
		}
	}
	return task.Conversation{}, false
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

func findConversationDot(db task.Database, conversationID, dotID string) (task.Dot, bool) {
	dot, ok := findDot(db, dotID)
	return dot, ok && dot.ConversationID == conversationID
}
