package desktopui

import (
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) chatView(c *ui.Context, theme *ui.Theme) {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.workspaceWelcomeView(c, theme)
		return
	}
	dotsByID := a.dotsByConversationID[conversation.ID]
	liveDotID := a.activeRunDotIDForConversation(conversation.ID)
	liveReply := a.liveReplies[liveDotID]
	if buffer := a.liveReplyBuffers[liveDotID]; buffer != nil {
		liveReply = buffer.String()
	}
	showLiveReply := liveReply != "" && dotsByID[liveDotID].ID != ""
	messageCount := len(conversation.Messages)
	if showLiveReply {
		messageCount++
	}
	a.chatMessages.FollowEnd = true
	a.chatMessages.Key = func(i int) any {
		if i < len(conversation.Messages) {
			return conversation.Messages[i].ID
		}
		return "live-reply-" + liveDotID
	}

	ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
		if len(conversation.Messages) == 0 && !showLiveReply {
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				emptyConversationFeed(c, theme, a, conversation)
			})
		} else {
			ui.List(c.Key("conversation-message-list"), &a.chatMessages, messageCount).
				Grow(1).FillWidth().MinHeight(0).Padding(16, 20, 18, 20).Gap(11).
				Rows(func(row ui.ListRow) {
					if row.Index < len(conversation.Messages) {
						message := conversation.Messages[row.Index]
						chatTimestamp(row.Context, theme, conversation.Messages, row.Index)
						conversationMessage(row.Context, theme, a, conversation, message, dotsByID)
						return
					}
					liveReplyBubble(row.Context, theme, a, conversation, liveDotID, liveReply)
				})
		}
		chatComposer(c, theme, a, conversation)
	})
}

func emptyConversationFeed(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation) {
	ui.Column(c).Fill().FillWidth().Center().Gap(13).Padding(28).Children(func() {
		profileAvatarForConversation(c, conversation, 50)
		ui.Column(c).MaxWidth(460).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Start with a prompt").FontSize(20).Bold()
			if strings.TrimSpace(conversation.Purpose) == "" {
				ui.Text(c, "Tell "+conversation.Name+" what you need. Give it a lasting role and working instructions from Customize.").
					FontSize(13).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
			} else {
				ui.Text(c, conversation.Purpose).FontSize(13).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
			}
		})
		if ui.Button(c.Key("empty-customize-profile"), "Customize profile").Padding(8, 12).Clicked() {
			a.openConversationProfile()
		}
	})
}

func chatTimestamp(c *ui.Context, theme *ui.Theme, messages []task.ConversationMessage, index int) {
	at := messages[index].CreatedAt.Local()
	if at.IsZero() {
		return
	}
	previous := time.Time{}
	if index > 0 {
		previous = messages[index-1].CreatedAt.Local()
	}
	y, m, d := at.Date()
	py, pm, pd := previous.Date()
	newDay := y != py || m != pm || d != pd
	if !newDay && at.Sub(previous) < 30*time.Minute {
		return
	}
	label := at.Format("3:04 PM")
	ny, nm, nd := time.Now().Local().Date()
	if newDay && (y != ny || m != nm || d != nd) {
		label = at.Format("Jan 2 · 3:04 PM")
	}
	ui.Text(c.Key("timestamp-"+messages[index].ID), label).FillWidth().TextAlign(ui.Center).
		FontSize(11).TextColor(theme.TextMuted).Padding(2, 0, 7, 0)
}

func conversationMessage(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation, message task.ConversationMessage, dots map[string]task.Dot) {
	if message.Role != "user" && message.Role != "assistant" && message.Role != "handoff" {
		return
	}
	dot := dots[message.DotID]
	isUser := message.Role == "user"
	rowKey := "message-row-" + message.ID
	bubbleKey := "message-bubble-" + message.ID
	if message.Role == "assistant" && dot.ID != "" {
		rowKey = "assistant-reply-row-" + dot.ID
		bubbleKey = "assistant-reply-bubble-" + dot.ID
	}
	speaker := conversation
	if message.SourceDotID != "" {
		if source, ok := findDot(a.snapshot, message.SourceDotID); ok {
			if sourceConversation, ok := findConversation(a.snapshot, source.ConversationID); ok {
				speaker = sourceConversation
			}
		}
	}
	if message.Role == "handoff" {
		rowKey = "handoff-row-" + message.ID
		bubbleKey = "handoff-bubble-" + message.ID
	}
	row := ui.Row(c.Key(rowKey)).FillWidth().AlignItems(ui.Start)
	row.Children(func() {
		if isUser {
			ui.Spacer(c)
		}
		bubble := chatBubble(c, bubbleKey)
		if isUser {
			bubble.Background(theme.Text).TextColor(contrastInk(theme.Text))
		} else {
			bubble.Background(theme.Surface)
		}
		bubble.Children(func() {
			if message.Role == "handoff" {
				chatSpeaker(c, theme, speaker, speaker.Name+" · specialist result")
			} else if !isUser && message.SourceDotID != "" {
				chatSpeaker(c, theme, speaker, speaker.Name)
			}
			if isUser {
				ink := contrastInk(theme.Text)
				if message.SourceDotID != "" {
					ui.Text(c, "Handoff from "+speaker.Name).FontSize(10).Bold().TextColor(ink)
				}
				ui.Text(c, message.Content).FontSize(14).TextColor(ink).FillWidth()
			} else {
				markdown.View(c, message.Content, a.markdownDisplayOptions()).FillWidth()
			}
		})
		if !isUser {
			ui.Spacer(c)
		}
	})
	if isUser {
		conversationRunActivity(c, theme, a, dot, message.ID)
	}
}

func liveReplyBubble(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation, dotID, draft string) {
	row := ui.Row(c.Key("assistant-reply-row-" + dotID)).FillWidth().AlignItems(ui.Start)
	row.Children(func() {
		bubble := chatBubble(c, "assistant-reply-bubble-"+dotID).
			Background(theme.Surface)
		bubble.Children(func() {
			chatSpeaker(c, theme, conversation, conversation.Name+" is replying")
			markdown.Stream(c, draft, a.markdownDisplayOptions()).FillWidth()
		})
		ui.Spacer(c)
	})
}

func chatBubble(c *ui.Context, key string) ui.Element {
	return ui.Column(c.Key(key)).MaxWidth(620).MinWidth(0).Gap(6).Padding(9, 12).Radius(workspacePanelRadius)
}

func chatSpeaker(c *ui.Context, theme *ui.Theme, speaker task.Conversation, label string) {
	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		profileAvatarForConversation(c, speaker, 16)
		ui.Text(c, label).SingleLine().MinWidth(0).FontSize(10).Bold().TextColor(theme.TextMuted)
	})
}

func (a *app) markdownDisplayOptions() *markdown.Options {
	if a.markdownOptions == nil {
		a.markdownOptions = &markdown.Options{
			WrapCode: true,
			Reveal:   &markdown.Reveal{Unit: markdown.ByWord, Fade: -1},
		}
	}
	return a.markdownOptions
}

func conversationRunActivity(c *ui.Context, theme *ui.Theme, a *app, dot task.Dot, messageID string) {
	if dot.ID == "" {
		return
	}
	activity := ""
	switch dot.Status {
	case task.Running:
		activity = "Thinking…"
		if strings.HasPrefix(dot.Checkpoint, "consulting ") {
			activity = "Consulting " + strings.TrimPrefix(dot.Checkpoint, "consulting ")
		}
	case task.Done:
		return
	case task.WaitingInput:
		activity = "Needs setup"
	case task.Failed:
		activity = "Could not reply"
	case task.Paused:
		activity = "Paused"
	default:
		activity = humanStatus(dot.Status)
	}
	ui.Row(c.Key("conversation-activity-"+dot.ID+"-"+messageID)).FillWidth().Gap(8).
		AlignItems(ui.Center).Padding(3, 0, 1, 28).Children(func() {
		ui.Icon(c, statusIcon(dot.Status)).Size(12, 12).TextColor(statusColor(theme, dot.Status))
		ui.Text(c, activity).FontSize(10).TextColor(theme.TextMuted)
		if dot.Status == task.WaitingInput {
			if ui.Button(c.Key("reply-settings-"+dot.ID), "Open settings").Clicked() {
				a.openSettings()
			}
		} else if dot.Status == task.Paused || dot.Status == task.Failed {
			if ui.Button(c.Key("retry-dot-"+dot.ID), "Retry").Clicked() {
				a.retryConversationMessage(dot.ID)
			}
		}
	})
}

func chatComposer(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation) {
	activeDotID := a.activeRunDotIDForConversation(conversation.ID)
	busy := activeDotID != ""
	footer := ui.Column(c).FillWidth().Gap(6).Padding(6, 20, 12, 20)
	footer.Children(func() {
		if a.conversationError != "" {
			ui.Text(c, a.conversationError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}
		if busy {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, iconPlay).Size(13, 13).TextColor(theme.Accent)
				label := conversation.Name + " is replying"
				ui.Text(c, label).FontSize(10).TextColor(theme.TextMuted).FillWidth()
				if ui.Button(c.Key("stop-conversation-run"), "Stop").Clicked() {
					a.stopConversationRun()
				}
			})
		}
		composer := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(5, 6, 5, 12).Radius(workspacePanelRadius).
			Background(theme.Surface).Border(1, theme.Border)
		composer.Children(func() {
			ui.TextAreaBase(c.Key("chat-message-"+conversation.ID), &a.messageDraft).
				Lines(1, 6).Grow(1).MinWidth(0).
				Placeholder("Message "+conversation.Name).Label("Message "+conversation.Name).
				OnShortcut(ui.Cmd, ui.KeyEnter, func() {
					if !busy && strings.TrimSpace(a.messageDraft) != "" {
						a.sendConversationMessage()
					}
				})
			button := appButton(c, theme, "send-conversation-message", "Send", buttonPrimary).
				Label("Send message").Tooltip("Send message · ⌘↵")
			button.Disabled(strings.TrimSpace(a.messageDraft) == "" || busy)
			if button.Clicked() {
				a.sendConversationMessage()
			}
		})
		if composer.FocusWithin() {
			composer.BorderColor(theme.Focus)
		}
	})
}

func boundedConversationPreview(content string, maxBytes int) (string, bool) {
	if len(content) <= maxBytes {
		return content, false
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(content[end]) {
		end--
	}
	return content[:end], true
}
