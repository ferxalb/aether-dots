package desktopui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) chatView(c *ui.Context, theme *ui.Theme) {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.workspaceWelcomeView(c, theme)
		return
	}
	childDots := conversationDots(a.snapshot, conversation.ID)
	dotsByID := make(map[string]task.Dot, len(childDots))
	for _, dot := range childDots {
		dotsByID[dot.ID] = dot
	}

	ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
		feed := ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).TrackScroll(&a.chatScroll)
		feed.Children(func() {
			if len(conversation.Messages) == 0 {
				emptyConversationFeed(c, theme, conversation)
				return
			}
			content := ui.Column(c).FillWidth().MinWidth(0).Gap(14).Padding(18, 24, 22, 24)
			content.Children(func() {
				for _, message := range conversation.Messages {
					conversationMessage(c, theme, a, conversation, message, dotsByID)
					if message.Role == "user" && message.DotID == a.runningDotID && a.live != "" {
						if dot, ok := dotsByID[message.DotID]; ok {
							liveDraftBubble(c, theme, dot, a.live)
						}
					}
				}
			})
		})

		chatComposer(c, theme, a, conversation)
	})
}

func emptyConversationFeed(c *ui.Context, theme *ui.Theme, conversation task.Conversation) {
	ui.Column(c).Fill().FillWidth().Center().Gap(10).Padding(24).Children(func() {
		avatar := ui.Box(c).Size(42, 42).Radius(15).
			Background(theme.Accent.Alpha(0.13)).TextColor(theme.Accent)
		avatar.Children(func() { ui.Icon(c, iconSparkles).Size(20, 20) })
		ui.Column(c).MaxWidth(400).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Message "+conversation.Name).FontSize(17).Bold()
			prompt := "Your first message starts a task using this Dot’s saved purpose and folders."
			if conversation.Mode == "local" {
				prompt += " Local extraction uses the purpose and current request, not earlier chat messages."
			} else {
				prompt += " OpenAI receives recent chat context and selected source text."
			}
			ui.Text(c, prompt).FontSize(12).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
		})
	})
}

func conversationMessage(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation, message task.ConversationMessage, dots map[string]task.Dot) {
	if message.Role == "system" {
		ui.Row(c.Key("system-message-row-" + message.ID)).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			text, _ := boundedConversationPreview(message.Content, 1000)
			ui.Text(c, text).MaxWidth(480).FillWidth().FontSize(11).TextColor(theme.TextMuted).TextAlign(ui.Center)
			ui.Spacer(c)
		})
		return
	}
	dot, hasDot := dots[message.DotID]
	isUser := message.Role == "user"
	row := ui.Row(c.Key("message-row-" + message.ID)).FillWidth().AlignItems(ui.Start)
	row.Children(func() {
		if isUser {
			ui.Spacer(c)
		}
		bubble := ui.Column(c.Key("message-bubble-"+message.ID)).MaxWidth(660).MinWidth(0).Gap(7).Padding(12, 15).Radius(17)
		if isUser {
			bubble.Background(theme.Accent).TextColor(theme.AccentText)
		} else {
			bubble.Background(theme.Surface).Border(1, theme.Border.Alpha(0.42))
		}
		bubble.Children(func() {
			if !isUser {
				label := conversation.Name
				if hasDot && dot.Mode == "local" {
					label = "Local extraction"
				}
				ui.Text(c, label).FontSize(10).Bold().TextColor(theme.TextMuted)
			}
			content, truncated := boundedConversationPreview(message.Content, 4000)
			ui.Text(c, content).FontSize(13)
			if truncated {
				ui.Text(c, "Preview limited to 4,000 bytes.").FontSize(11).TextColor(theme.TextMuted)
			}
			if !isUser && hasDot && len(dot.Artifacts) > 0 {
				artifact := dot.Artifacts[len(dot.Artifacts)-1]
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Icon(c, iconCheck).Size(13, 13).TextColor(theme.Success)
					ui.Text(c, "Saved locally · "+filepath.Base(artifact.Path)).
						FontSize(10).TextColor(theme.TextMuted).SingleLine().Tooltip(artifact.Path).FillWidth()
					open := ui.Button(c.Key("open-chat-artifact-"+message.ID+"-"+artifact.SHA256), "Open file").
						Label("Open verified local result")
					if open.Clicked() {
						a.openVerifiedConversationArtifact(artifact.Path)
					}
				})
			}
		})
		if !isUser {
			ui.Spacer(c)
		}
	})

	if isUser && hasDot {
		conversationRunActivity(c, theme, a, dot, message.ID)
	}
}

func liveDraftBubble(c *ui.Context, theme *ui.Theme, dot task.Dot, draft string) {
	row := ui.Row(c.Key("live-draft-row-" + dot.ID)).FillWidth().AlignItems(ui.Start)
	row.Children(func() {
		bubble := ui.Column(c.Key("live-draft-bubble-"+dot.ID)).MaxWidth(660).MinWidth(0).
			Gap(6).Padding(12, 15).Radius(17).Background(theme.Surface).Border(1, theme.Border)
		bubble.Children(func() {
			ui.Text(c, "Live model draft").FontSize(10).Bold().TextColor(theme.TextMuted)
			preview, _ := boundedConversationPreview(draft, 4000)
			ui.Text(c, preview).FontSize(13)
		})
		ui.Spacer(c)
	})
}

func conversationRunActivity(c *ui.Context, theme *ui.Theme, a *app, dot task.Dot, messageID string) {
	activity := ""
	if dot.Status == task.Running {
		activity = runPhase(dot)
	} else if len(dot.Events) > 0 {
		activity = tail(dot.Events[len(dot.Events)-1].Message, 260)
	}
	if dot.Status == task.Done && len(dot.Artifacts) > 0 {
		activity = "Completed · output verified locally."
	}
	if activity == "" {
		activity = "Task " + humanStatus(dot.Status) + "."
	}
	activityRow := ui.Row(c.Key("conversation-activity-"+dot.ID+"-"+messageID)).FillWidth().Gap(8).
		AlignItems(ui.Center).Padding(2, 0, 2, 26).Wrap()
	activityRow.Children(func() {
		ui.Icon(c, statusIcon(dot.Status)).Size(13, 13).TextColor(statusColor(theme, dot.Status))
		ui.Text(c, activity).FontSize(11).TextColor(theme.TextMuted).Grow(1).MinWidth(0).FillWidth()
		canResume := dot.Status == task.Paused || dot.Status == task.Failed ||
			dot.Status == task.WaitingInput || dot.Status == task.Captured || dot.Status == task.Ready
		if canResume {
			resume := ui.Button(c.Key("resume-conversation-dot-"+dot.ID+"-"+messageID), "Resume")
			resume.Disabled(a.cancelRun != nil)
			if resume.Clicked() {
				a.startRun(dot.ID)
			}
		}
		if dot.Status == task.WaitingInput && dot.Mode == "model" {
			if ui.Button(c.Key("open-settings-"+dot.ID), "OpenAI settings").Clicked() {
				a.openSettings()
			}
			if ui.Button(c.Key("switch-local-"+dot.ID), "Switch to local").Clicked() {
				if err := a.db.SetMode(dot.ID, "local"); err != nil {
					a.conversationError = err.Error()
				} else {
					a.refresh()
				}
			}
		}
	})
}

func runPhase(dot task.Dot) string {
	checkpoint := strings.ToLower(dot.Checkpoint)
	switch {
	case strings.Contains(checkpoint, "reading selected text files"):
		return "Reading selected files"
	case strings.Contains(checkpoint, "composing"):
		return "Preparing a draft"
	case strings.Contains(checkpoint, "writing and verifying"):
		return "Writing and verifying the local result"
	case strings.Contains(checkpoint, "checking model configuration"):
		return "Checking model configuration"
	case dot.Mode == "model":
		return "Preparing an OpenAI draft"
	default:
		return "Running local extraction"
	}
}

func chatComposer(c *ui.Context, theme *ui.Theme, a *app, conversation task.Conversation) {
	busy := a.cancelRun != nil
	activeDot, activeDotOK := findDot(a.snapshot, a.runningDotID)
	footer := ui.Column(c).FillWidth().Gap(6).Padding(10, 20, 12, 20).
		Background(theme.Background).Border(1, theme.Border)
	footer.Children(func() {
		if a.conversationError != "" {
			ui.Text(c, a.conversationError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}
		if busy {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, iconPlay).Size(14, 14).TextColor(theme.Accent)
				message := "A Dot is running"
				if activeDotOK && activeDot.ConversationID != conversation.ID {
					message = "Another conversation is running"
				}
				ui.Text(c, message).FontSize(11).TextColor(theme.TextMuted).FillWidth()
				stop := ui.Button(c.Key("stop-conversation-run"), "Stop").Label("Stop the active task")
				if stop.Clicked() {
					a.stopConversationRun()
				}
			})
		}

		composer := ui.Row(c).FillWidth().AlignItems(ui.End).Gap(8).Padding(8, 10, 8, 12).Radius(18).
			Background(theme.Surface).Border(1, theme.Border)
		composer.Children(func() {
			ui.TextAreaBase(c.Key("chat-message-"+conversation.ID), &a.messageDraft).
				MinHeight(42).MaxHeight(110).Grow(1).MinWidth(0).
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
		ui.Text(c, "⌘↵ to send · Shift+↵ for a new line").FontSize(10).TextColor(theme.TextMuted)
	})
}

func (a *app) conversationDetailsView(c *ui.Context, theme *ui.Theme) {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.workspaceWelcomeView(c, theme)
		return
	}
	dots := conversationDots(a.snapshot, conversation.ID)
	ui.Column(c).FillWidth().MaxWidth(840).MinWidth(0).Gap(14).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Spacer(c)
			ui.Badge(c, selectedSpaceName(a.snapshot, conversation.SpaceID))
		})
		if a.conversationError != "" {
			ui.Text(c, a.conversationError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, conversation.Name).FontSize(21).Bold()
			ui.Text(c, "Each message starts a task using this Dot’s purpose and saved folders.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth()
		})
		taskPanel(c, theme, "Purpose and execution", func() {
			ui.Text(c, conversation.Purpose).FontSize(13).FillWidth()
			ui.Divider(c)
			mode := "Local extraction"
			if conversation.Mode == "model" {
				mode = "OpenAI model · " + a.prefs.Model
			}
			ui.Text(c, mode).FontSize(12).TextColor(theme.TextMuted)
			ui.Text(c, executionDisclosureFor(conversation.Mode, a.prefs.Model)).
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		taskPanel(c, theme, "Local permissions", func() {
			taskScopeLine(c, theme, "Read only", conversation.SourceRoot)
			taskScopeLine(c, theme, "Write only", conversation.OutputDir)
		})
		taskPanel(c, theme, fmt.Sprintf("Tasks · %d", len(dots)), func() {
			if len(dots) == 0 {
				ui.Text(c, "No tasks yet. Send the first message in this Dot’s chat.").
					FontSize(12).TextColor(theme.TextMuted).FillWidth()
			}
			for _, dot := range dots {
				ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(9).Children(func() {
					ui.Icon(c, statusIcon(dot.Status)).Size(14, 14).TextColor(statusColor(theme, dot.Status))
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Text(c, dot.Name).FontSize(12).Bold().FillWidth()
							ui.Badge(c, humanStatus(dot.Status))
						})
						if len(dot.Artifacts) > 0 {
							artifact := dot.Artifacts[len(dot.Artifacts)-1]
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
								ui.Text(c, "Verified · "+artifact.Path).FontSize(10).TextColor(theme.TextMuted).
									SingleLine().Tooltip(artifact.Path).FillWidth()
								open := ui.Button(c.Key("details-open-"+dot.ID+"-"+artifact.SHA256), "Open file").
									Label("Open verified local result")
								if open.Clicked() {
									a.openVerifiedConversationArtifact(artifact.Path)
								}
							})
						}
					})
				})
			}
		})
		taskPanel(c, theme, fmt.Sprintf("Routines · %d", len(a.snapshot.Routines)), func() {
			if len(a.snapshot.Routines) == 0 {
				ui.Text(c, "Routines saved from completed Dots will appear here.").
					FontSize(12).TextColor(theme.TextMuted).FillWidth()
			}
			for _, routine := range a.snapshot.Routines {
				button := ui.Button(c.Key("conversation-routine-"+routine.ID), "").
					FillWidth().Padding(7, 8).Radius(8).Label(routine.Name)
				if button.Hovered() {
					button.Background(theme.SurfaceHover)
				}
				button.Children(func() {
					ui.Column(c).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, routine.Name).FontSize(12).Bold().FillWidth()
						ui.Text(c, "Edit or run with the Routine’s original permissions").
							FontSize(10).TextColor(theme.TextMuted).FillWidth()
					})
				})
				if button.Clicked() {
					a.selectedRoutineID = routine.ID
					a.routineName = routine.Name
					a.routineObjective = routine.Objective
					a.conversationDetailsOpen = false
				}
			}
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
