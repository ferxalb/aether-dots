package desktopui

import (
	"strings"
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
	dots := conversationDots(a.snapshot, conversation.ID)
	dotsByID := make(map[string]task.Dot, len(dots))
	for _, dot := range dots {
		dotsByID[dot.ID] = dot
	}
	assistantReplies := make(map[string]bool, len(conversation.Messages))
	for _, message := range conversation.Messages {
		if message.Role == "assistant" {
			assistantReplies[message.DotID] = true
		}
	}

	ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
		feed := ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).TrackScroll(&a.chatScroll)
		feed.Children(func() {
			if len(conversation.Messages) == 0 {
				emptyConversationFeed(c, theme, a, conversation)
				return
			}
			ui.Column(c).FillWidth().MinWidth(0).Gap(14).Padding(20, 24, 24, 24).Children(func() {
				for _, message := range conversation.Messages {
					conversationMessage(c, theme, a, conversation, message, dotsByID)
					if message.Role == "user" && message.DotID == a.runningDotID && a.live != "" && !assistantReplies[message.DotID] {
						liveReplyBubble(c, theme, a, conversation, message.DotID, a.live)
					}
				}
			})
		})
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

func conversationMessage(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation, message task.ConversationMessage, dots map[string]task.Dot) {
	if message.Role != "user" && message.Role != "assistant" {
		return
	}
	dot := dots[message.DotID]
	isUser := message.Role == "user"
	rowKey := "message-row-" + message.ID
	bubbleKey := "message-bubble-" + message.ID
	if !isUser && dot.ID != "" {
		rowKey = "assistant-reply-row-" + dot.ID
		bubbleKey = "assistant-reply-bubble-" + dot.ID
	}
	row := ui.Row(c.Key(rowKey)).FillWidth().AlignItems(ui.Start)
	row.Children(func() {
		if isUser {
			ui.Spacer(c)
		}
		bubble := ui.Column(c.Key(bubbleKey)).MaxWidth(720).MinWidth(0).
			Gap(7).Padding(12, 15).Radius(16)
		if isUser {
			bubble.Background(theme.Accent).TextColor(theme.AccentText)
		} else {
			bubble.Background(theme.Surface).Border(1, theme.Border.Alpha(0.55))
		}
		bubble.Children(func() {
			if !isUser {
				ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
					profileAvatarForConversation(c, conversation, 20)
					ui.Text(c, conversation.Name).FontSize(10).Bold().TextColor(theme.TextMuted)
				})
			}
			if isUser {
				ui.Text(c, message.Content).FontSize(13).FillWidth()
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
		bubble := ui.Column(c.Key("assistant-reply-bubble-"+dotID)).MaxWidth(720).MinWidth(0).
			Gap(7).Padding(12, 15).Radius(16).Background(theme.Surface).Border(1, theme.Border)
		bubble.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
				profileAvatarForConversation(c, conversation, 20)
				ui.Text(c, conversation.Name+" is replying").FontSize(10).Bold().TextColor(theme.TextMuted)
			})
			markdown.Stream(c, draft, a.markdownDisplayOptions()).FillWidth()
		})
		ui.Spacer(c)
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
	case task.Done:
		activity = "Replied"
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
	busy := a.cancelRun != nil
	activeDot, activeDotOK := findDot(a.snapshot, a.runningDotID)
	footer := ui.Column(c).FillWidth().Gap(6).Padding(8, 24, 16, 24)
	footer.Children(func() {
		if a.conversationError != "" {
			ui.Text(c, a.conversationError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}
		if busy {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, iconPlay).Size(13, 13).TextColor(theme.Accent)
				label := conversation.Name + " is replying"
				if activeDotOK && activeDot.ConversationID != conversation.ID {
					label = "Another Dot is replying"
				}
				ui.Text(c, label).FontSize(10).TextColor(theme.TextMuted).FillWidth()
				if ui.Button(c.Key("stop-conversation-run"), "Stop").Clicked() {
					a.stopConversationRun()
				}
			})
		}
		composer := ui.Row(c).FillWidth().AlignItems(ui.End).Gap(8).Padding(9, 12, 9, 14).Radius(24).
			Background(theme.Surface).Border(1, theme.Border.Alpha(0.48)).
			Shadow(0, 0, 0, 0, ui.RGBA(0, 0, 0, 0.14))
		composer.Children(func() {
			ui.TextAreaBase(c.Key("chat-message-"+conversation.ID), &a.messageDraft).
				MinHeight(38).MaxHeight(120).Grow(1).MinWidth(0).
				Placeholder("Message "+conversation.Name).Label("Message "+conversation.Name).
				OnShortcut(ui.Cmd, ui.KeyEnter, func() {
					if !busy && strings.TrimSpace(a.messageDraft) != "" {
						a.sendConversationMessage()
					}
				})
			button := ui.PrimaryButton(c.Key("send-conversation-message"), "Send").
				Label("Send message").Tooltip("Send message · ⌘↵")
			button.Disabled(strings.TrimSpace(a.messageDraft) == "" || busy)
			if button.Clicked() {
				a.sendConversationMessage()
			}
		})
		if composer.FocusWithin() {
			composer.BorderColor(theme.Focus)
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "⌘↵ to send · Shift+↵ for a new line").FontSize(10).TextColor(theme.TextMuted)
		})
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
