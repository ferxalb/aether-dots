package desktopui

import (
	"context"
	"errors"
	"strings"

	"aether-dots/internal/model"
	"aether-dots/internal/task"
)

func (a *app) compactConversationHistory() {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		return
	}
	if len(conversation.Messages) == 0 {
		a.conversationError = "Send a few messages before creating chat memory."
		return
	}
	if a.summaryCompactRun == nil {
		a.summaryCompactRun = make(map[string]bool)
	}
	if a.summaryCompactRun[conversation.ID] {
		return
	}
	a.summaryCompactRun[conversation.ID] = true
	a.conversationError = ""
	a.adt.Loading("compact-history-"+conversation.ID, "Building Dot memory", "The selected provider will summarize recent chat into editable local notes.")
	prefs := a.prefs
	win := a.win
	originalDraft := a.profileContextSummary
	input := compactConversationPrompt(conversation)
	go func() {
		provider := model.WithoutNativeTools(generatorForPreferences(prefs))
		provider = model.WithMaxOutputTokens(provider, 2048)
		ctx, cancel := context.WithTimeout(context.Background(), secondsPreference(prefs.TimeoutSeconds, 90))
		defer cancel()
		summary, err := provider.Generate(ctx, input, nil)
		summary = strings.TrimSpace(summary)
		if err == nil && len(summary) > task.MaxConversationSummaryBytes {
			err = errors.New("the generated summary exceeds the 8 KiB limit; edit or shorten it before saving")
		}
		if err == nil {
			err = a.db.SetConversationContextSummary(conversation.ID, summary)
		}
		finish := func() {
			delete(a.summaryCompactRun, conversation.ID)
			a.refresh()
			if err != nil {
				a.adt.Push(ADTNotice{ID: "compact-history-" + conversation.ID, Title: "Couldn’t build Dot memory", Description: err.Error(), Level: ADTError})
				return
			}
			if a.conversationDetailsOpen && a.selectedConversationID == conversation.ID && a.profileContextSummary == originalDraft {
				a.profileContextSummary = summary
			}
			a.adt.Push(ADTNotice{ID: "compact-history-" + conversation.ID, Title: "Dot memory updated", Description: "The full transcript remains on this device; the new summary can be edited before it is sent again.", Level: ADTSuccess})
		}
		if win == nil {
			finish()
			return
		}
		win.Update(finish)
	}()
}

func compactConversationPrompt(conversation task.Conversation) string {
	var b strings.Builder
	b.WriteString("Update a concise persistent working-memory summary for this Dot. Keep durable user preferences, project facts, decisions, terminology, open questions, and next steps. Drop greetings, repeated context, and details that are no longer useful. Preserve uncertainty and distinguish user-provided facts from assumptions. Return only the updated memory text, with clear headings where helpful. Do not invent missing details.\n\n")
	if strings.TrimSpace(conversation.ContextSummary) != "" {
		b.WriteString("Existing memory (update this, do not blindly repeat it):\n")
		b.WriteString(conversation.ContextSummary)
		b.WriteString("\n\n")
	}
	b.WriteString("Recent saved conversation (older transcript remains on device):\n")
	eligible := make([]task.ConversationMessage, 0, task.MaxConversationContextMessages)
	for i := len(conversation.Messages) - 1; i >= 0 && len(eligible) < task.MaxConversationContextMessages; i-- {
		message := conversation.Messages[i]
		if message.Role == "user" || message.Role == "assistant" || message.Role == "handoff" {
			eligible = append(eligible, message)
		}
	}
	for i := len(eligible) - 1; i >= 0; i-- {
		message := eligible[i]
		label := "User"
		if message.Role == "assistant" {
			label = "Assistant"
		} else if message.Role == "handoff" {
			label = "Specialist"
		}
		b.WriteString("\n")
		b.WriteString(label)
		b.WriteString(": ")
		remaining := task.MaxConversationContextBytes - b.Len()
		if remaining <= 0 {
			break
		}
		b.WriteString(truncateUTF8Bytes(message.Content, remaining))
		if b.Len() >= task.MaxConversationContextBytes {
			break
		}
	}
	return b.String()
}
