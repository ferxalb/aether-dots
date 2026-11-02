package desktopui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) skillLibrarySettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(13).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "Skills").FontSize(15).Bold()
				ui.Text(c, "Save reusable instructions and choose when a Dot should use them.").
					FontSize(12).TextColor(theme.TextMuted).FillWidth()
			})
			if !a.skillEditorOpen {
				if ui.PrimaryButton(c.Key("skill-new"), "New skill").Padding(8, 12).Clicked() {
					a.startSkillEditor()
				}
			}
		})
		if len(a.snapshot.Skills) == 0 && !a.skillEditorOpen {
			ui.Column(c).FillWidth().Gap(5).Padding(15, 14).Radius(11).
				Background(theme.Background).Children(func() {
				ui.Text(c, "Nothing saved yet").FontSize(12).Bold()
				ui.Text(c, "Create a skill for a task you repeat. It will be available to all your Dots.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
		}
		for _, skill := range a.snapshot.Skills {
			ui.Row(c.Key("skill-row-"+skill.ID)).FillWidth().AlignItems(ui.Center).Gap(9).
				Padding(9, 11).Radius(10).Background(theme.Background).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, skill.Name).FontSize(12).Bold().SingleLine()
					ui.Text(c, skill.Description).FontSize(10).TextColor(theme.TextMuted).FillWidth()
				})
				if ui.Button(c.Key("skill-edit-"+skill.ID), "Edit").Padding(7, 10).Clicked() {
					a.editSkill(skill)
				}
				if a.skillDeleteID == skill.ID {
					if ui.Button(c.Key("skill-delete-confirm-"+skill.ID), "Delete skill").Padding(7, 10).Clicked() {
						if err := a.db.DeleteSkill(skill.ID); err != nil {
							a.adt.Error("Couldn't remove the skill", err.Error())
						} else {
							a.adt.Success("Skill removed", "Saved chat history keeps its original instructions.")
							a.skillEditingID, a.skillDeleteID = "", ""
							a.skillEditorOpen = false
							a.skillAdvancedOpen = false
							a.skillDraft = task.Skill{}
							a.refresh()
						}
					}
				} else if ui.Button(c.Key("skill-delete-"+skill.ID), "Remove").Padding(7, 10).Clicked() {
					a.skillDeleteID = skill.ID
				}
			})
		}
		if len(a.snapshot.Skills) > 0 {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "@skill[Name]").FontSize(11).Bold().TextColor(theme.Accent)
				ui.Text(c, "in a message uses a skill for that reply.").FontSize(11).TextColor(theme.TextMuted)
			})
		}
		if a.skillEditorOpen {
			ui.Divider(c)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				title := "New skill"
				if a.skillEditingID != "" {
					title = "Edit skill"
				}
				ui.Text(c, title).FontSize(13).Bold()
				ui.Spacer(c)
				if ui.Button(c.Key("skill-cancel-edit"), "Cancel").Padding(7, 10).Clicked() {
					a.skillEditingID, a.skillDeleteID = "", ""
					a.skillEditorOpen = false
					a.skillAdvancedOpen = false
					a.skillDraft = task.Skill{}
				}
			})
			ui.Field(c, "Name", func() {
				ui.TextInput(c.Key("skill-name"), &a.skillDraft.Name).Placeholder("Weekly account health").FillWidth()
			})
			ui.Field(c, "Description", func() {
				ui.TextInput(c.Key("skill-description"), &a.skillDraft.Description).Placeholder("What result does this skill help create?").FillWidth()
			})
			skillTextArea(c, "skill-when", "When to use", &a.skillDraft.WhenToUse, "Describe when a Dot should use this skill.")
			skillTextArea(c, "skill-steps", "Steps and decision rules", &a.skillDraft.Steps, "Write the workflow in order and explain important decisions.")
			skillTextArea(c, "skill-output", "What the answer should include", &a.skillDraft.Output, "Describe the format and key information to include.")
			a.skillAdvancedOptions(c, theme)
			if a.skillAdvancedOpen {
				skillTextArea(c, "skill-inputs", "Required information", &a.skillDraft.Inputs, "List the context or details the Dot needs. Say what to do if something is missing.")
				skillTextArea(c, "skill-validation", "How to check the result", &a.skillDraft.Validation, "Explain how to check the answer and its sources.")
				skillTextArea(c, "skill-approval", "When to ask first", &a.skillDraft.Approval, "Describe actions that need your approval and when the Dot should stop.")
			}
		}
	})
}

func skillTextArea(c *ui.Context, key, label string, value *string, placeholder string) {
	ui.Field(c, label, func() {
		ui.TextArea(c.Key(key), value).MinHeight(52).MaxHeight(140).FillWidth().
			Placeholder(placeholder)
	})
}

func (a *app) skillAdvancedOptions(c *ui.Context, theme *ui.Theme) {
	label := "Show more guidance"
	if a.skillAdvancedOpen {
		label = "Hide extra guidance"
	}
	button := ui.Button(c.Key("skill-advanced-toggle"), "").FillWidth().Padding(10, 12).
		Gap(10).Radius(9).Background(theme.Background).Border(1, theme.Border).
		Label(label)
	if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
	button.Children(func() {
		ui.Text(c, label).FontSize(11).FillWidth()
		chevron := "⌄"
		if a.skillAdvancedOpen {
			chevron = "⌃"
		}
		ui.Text(c, chevron).FontSize(13).TextColor(theme.TextMuted)
	})
	if button.Clicked() {
		a.skillAdvancedOpen = !a.skillAdvancedOpen
	}
}

func (a *app) editSkill(skill task.Skill) {
	a.skillEditingID = skill.ID
	a.skillEditorOpen = true
	a.skillAdvancedOpen = skill.Inputs != "" || skill.Validation != "" || skill.Approval != ""
	a.skillDeleteID = ""
	a.skillDraft = skill
}

func (a *app) startSkillEditor() {
	a.skillEditingID, a.skillDeleteID = "", ""
	a.skillEditorOpen = true
	a.skillAdvancedOpen = false
	a.skillDraft = task.Skill{}
}

func (a *app) saveSkill() {
	in := task.CreateSkill{
		Name: a.skillDraft.Name, Description: a.skillDraft.Description,
		WhenToUse: a.skillDraft.WhenToUse, Inputs: a.skillDraft.Inputs,
		Steps: a.skillDraft.Steps, Validation: a.skillDraft.Validation,
		Output: a.skillDraft.Output, Approval: a.skillDraft.Approval,
	}
	if a.skillEditingID == "" {
		skill, err := a.db.CreateSkill(in)
		if err != nil {
			a.adt.Error("Couldn't save the skill", err.Error())
			return
		}
		a.skillEditingID, a.skillDraft = skill.ID, skill
		a.adt.Success("Skill saved", "Use @skill["+skill.Name+"] in a message to apply it.")
	} else {
		if err := a.db.UpdateSkill(a.skillEditingID, in); err != nil {
			a.adt.Error("Couldn't update the skill", err.Error())
			return
		}
		a.adt.Success("Skill updated", "Your changes are ready to use.")
	}
	a.refresh()
}

func (a *app) routineSettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(13).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "Routines").FontSize(15).Bold()
				ui.Text(c, "Have a Dot repeat a prompt on a schedule. Results appear in its conversation.").
					FontSize(12).TextColor(theme.TextMuted).FillWidth()
			})
			if !a.routineEditorOpen {
				if ui.PrimaryButton(c.Key("routine-new"), "New routine").Padding(8, 12).Clicked() {
					a.startRoutineEditor()
				}
			}
		})
		if len(a.snapshot.Routines) == 0 && !a.routineEditorOpen {
			ui.Column(c).FillWidth().Gap(5).Padding(15, 14).Radius(11).
				Background(theme.Background).Children(func() {
				ui.Text(c, "Nothing scheduled yet").FontSize(12).Bold()
				ui.Text(c, "Create a routine to run a prompt for one of your Dots while AETHER is open.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
		}
		for _, routine := range a.snapshot.Routines {
			conversationName := "Missing Dot"
			if conversation, ok := findConversation(a.snapshot, routine.ConversationID); ok {
				conversationName = conversation.Name
			}
			ui.Column(c.Key("routine-row-"+routine.ID)).FillWidth().Gap(7).Padding(10, 11).Radius(10).
				Background(theme.Background).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						state := "Paused"
						if routine.Active {
							state = "Active"
						}
						ui.Text(c, routine.Name+" · "+state).FontSize(12).Bold().SingleLine()
						ui.Text(c, conversationName+" · "+routineIntervalSummary(routine.IntervalSeconds)).
							FontSize(10).TextColor(theme.TextMuted).SingleLine()
					})
					if ui.Button(c.Key("routine-edit-"+routine.ID), "Edit").Padding(6, 9).Clicked() {
						a.editRoutine(routine)
					}
					label := "Pause"
					if !routine.Active {
						label = "Resume"
					}
					if ui.Button(c.Key("routine-toggle-"+routine.ID), label).Padding(6, 9).Clicked() {
						if err := a.db.UpdateRoutineActive(routine.ID, !routine.Active); err != nil {
							a.adt.Error("Couldn't update the routine", err.Error())
						} else {
							if routine.Active {
								a.adt.Info("Routine paused", "It won't run again until you resume it.")
							} else {
								a.adt.Success("Routine resumed", "The next run has been scheduled.")
							}
							a.refresh()
						}
					}
				})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					status := "Paused"
					if routine.Active {
						status = "Next run " + routine.NextRunAt.Local().Format("Jan 2, 3:04 PM")
					}
					if routine.LastStatus != "" {
						status += " · last " + routine.LastStatus
					}
					ui.Text(c, status).FontSize(10).TextColor(theme.TextMuted).FillWidth()
					if ui.Button(c.Key("routine-test-"+routine.ID), "Run now").Padding(6, 9).
						Tooltip("Run once now. This sends a request to OpenAI and may use your plan.").
						Disabled(a.cancelRun != nil).Clicked() {
						a.testRoutine(routine.ID)
					}
					if a.routineDeleteID == routine.ID {
						if ui.Button(c.Key("routine-delete-confirm-"+routine.ID), "Confirm delete").Padding(6, 9).Clicked() {
							if err := a.db.DeleteRoutine(routine.ID); err != nil {
								a.adt.Error("Couldn't delete the routine", err.Error())
							} else {
								a.adt.Success("Routine deleted", "It won't run again.")
								a.routineEditingID, a.routineDeleteID = "", ""
								a.routineEditorOpen = false
								a.refresh()
							}
						}
					} else if ui.Button(c.Key("routine-delete-"+routine.ID), "Delete").Padding(6, 9).Clicked() {
						a.routineDeleteID = routine.ID
					}
				})
				start := max(0, len(routine.Runs)-3)
				for i := len(routine.Runs) - 1; i >= start; i-- {
					run := routine.Runs[i]
					label := run.Status + " · " + run.StartedAt.Local().Format("Jan 2, 3:04 PM")
					if run.Message != "" {
						message, _ := boundedConversationPreview(run.Message, 160)
						label += " · " + message
					}
					ui.Text(c, label).FontSize(10).TextColor(theme.TextMuted).FillWidth()
				}
			})
		}
		if a.routineEditorOpen {
			ui.Divider(c)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				title := "New routine"
				if a.routineEditingID != "" {
					title = "Edit routine"
				}
				ui.Text(c, title).FontSize(13).Bold()
				ui.Spacer(c)
				if ui.Button(c.Key("routine-cancel-edit"), "Cancel").Padding(7, 10).Clicked() {
					a.routineEditingID, a.routineDeleteID = "", ""
					a.routineEditorOpen = false
					a.routineNameDraft, a.routinePromptDraft = "", ""
					a.routineEveryDraft, a.routineCustomDraft, a.routineDotDraft = "1h", "", a.selectedConversationID
				}
			})
			ui.Field(c, "Dot", func() { conversationChoice(c, theme, a) })
			ui.Field(c, "Name", func() {
				ui.TextInput(c.Key("routine-name"), &a.routineNameDraft).Placeholder("Daily review").FillWidth()
			})
			ui.Field(c, "Run every", func() {
				routineFrequencyChoice(c, theme, a)
			})
			if a.routineEveryDraft == "custom" {
				ui.Field(c, "Custom interval", func() {
					ui.TextInput(c.Key("routine-custom-every"), &a.routineCustomDraft).
						Placeholder("e.g. 90m, 2h, or 2h30m").FillWidth()
				})
				ui.Text(c, "Use minutes or hours. The minimum interval is 5 minutes and the maximum is 1 year.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			ui.Field(c, "Instruction", func() {
				ui.TextArea(c.Key("routine-prompt"), &a.routinePromptDraft).MinHeight(74).MaxHeight(180).FillWidth().
					Placeholder("Review the current conversation, note anything missing, and return a concise summary.")
			})
			ui.Text(c, "Intervals start after you create or resume a routine (minimum 5 minutes). Run now sends a request to OpenAI and may use your plan. Schedules run while the desktop app is open; missed intervals are skipped.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
	})
}

func conversationChoice(c *ui.Context, theme *ui.Theme, a *app) {
	selectConversation := ui.SelectBase(c.Key("routine-dot"), &a.routineDotDraft)
	selectConversation.Trigger.FillWidth().MinWidth(0).Padding(8, 10).Radius(9).
		Background(theme.Background).Border(1, theme.Border).Children(func() {
		name := "Choose a Dot"
		if conversation, ok := findConversation(a.snapshot, a.routineDotDraft); ok {
			name = conversation.Name
		}
		ui.Text(c, name).FontSize(12).FillWidth().SingleLine()
		ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
	})
	selectConversation.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(theme.Surface).Border(1, theme.Border).
			Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
		for _, conversation := range a.snapshot.Conversations {
			item := selectConversation.Item(conversation.ID).FillWidth().Padding(9, 10).Radius(7)
			if item.Highlighted() || conversation.ID == a.routineDotDraft {
				item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
			}
			item.Children(func() { ui.Text(c, conversation.Name).FontSize(12) })
		}
	})
}

func routineFrequencyChoice(c *ui.Context, theme *ui.Theme, a *app) {
	selectFrequency := ui.SelectBase(c.Key("routine-every"), &a.routineEveryDraft)
	selectFrequency.Trigger.FillWidth().MinWidth(0).Padding(8, 10).Radius(9).
		Background(theme.Background).Border(1, theme.Border).Children(func() {
		ui.Text(c, routineFrequencyLabel(a.routineEveryDraft)).FontSize(12).FillWidth().SingleLine()
		ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
	})
	selectFrequency.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(10).Background(theme.Surface).Border(1, theme.Border).
			Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
		for _, value := range []string{"5m", "15m", "30m", "1h", "4h", "8h", "12h", "24h", "custom"} {
			item := selectFrequency.Item(value).FillWidth().Padding(9, 10).Radius(7)
			if item.Highlighted() || value == a.routineEveryDraft {
				item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
			}
			item.Children(func() { ui.Text(c, routineFrequencyLabel(value)).FontSize(12) })
		}
	})
}

func routineFrequencyLabel(value string) string {
	switch value {
	case "5m":
		return "Every 5 minutes"
	case "15m":
		return "Every 15 minutes"
	case "30m":
		return "Every 30 minutes"
	case "1h":
		return "Every hour"
	case "4h":
		return "Every 4 hours"
	case "8h":
		return "Every 8 hours"
	case "12h":
		return "Every 12 hours"
	case "24h":
		return "Daily"
	case "custom":
		return "Custom interval…"
	default:
		return value
	}
}

func routineIntervalSummary(seconds int64) string {
	duration := time.Duration(seconds) * time.Second
	if duration%(24*time.Hour) == 0 {
		days := int(duration / (24 * time.Hour))
		if days == 1 {
			return "Daily"
		}
		return fmt.Sprintf("Every %d days", days)
	}
	if duration%time.Hour == 0 {
		hours := int(duration / time.Hour)
		if hours == 1 {
			return "Every hour"
		}
		return fmt.Sprintf("Every %d hours", hours)
	}
	if duration%time.Minute == 0 {
		minutes := int(duration / time.Minute)
		return fmt.Sprintf("Every %d minutes", minutes)
	}
	return fmt.Sprintf("Every %d seconds", int(duration/time.Second))
}

func (a *app) editRoutine(routine task.Routine) {
	a.routineEditingID = routine.ID
	a.routineEditorOpen = true
	a.routineDeleteID = ""
	a.routineNameDraft = routine.Name
	a.routinePromptDraft = routine.Prompt
	a.routineDotDraft = routine.ConversationID
	interval := task.FormatRoutineInterval(routine.IntervalSeconds)
	if isPresetRoutineInterval(interval) {
		a.routineEveryDraft, a.routineCustomDraft = interval, ""
	} else {
		a.routineEveryDraft, a.routineCustomDraft = "custom", interval
	}
}

func (a *app) startRoutineEditor() {
	a.routineEditingID, a.routineDeleteID = "", ""
	a.routineEditorOpen = true
	a.routineNameDraft, a.routinePromptDraft = "", ""
	a.routineDotDraft, a.routineEveryDraft, a.routineCustomDraft = a.selectedConversationID, "1h", ""
}

func (a *app) saveRoutine() {
	intervalValue := a.routineEveryDraft
	if intervalValue == "custom" {
		intervalValue = a.routineCustomDraft
	}
	interval, err := time.ParseDuration(strings.TrimSpace(intervalValue))
	if err != nil {
		a.adt.Warning("Choose a valid schedule", "Pick one of the listed intervals or enter a duration such as 90m or 2h.")
		return
	}
	if interval < 5*time.Minute {
		a.adt.Warning("Interval is too short", "Routines can run as often as every 5 minutes.")
		return
	}
	if interval > 365*24*time.Hour {
		a.adt.Warning("Interval is too long", "The longest allowed schedule is one year.")
		return
	}
	in := task.CreateRoutine{
		ConversationID: a.routineDotDraft, Name: a.routineNameDraft,
		Prompt: a.routinePromptDraft, IntervalSeconds: int64(interval / time.Second),
	}
	if a.routineEditingID == "" {
		routine, err := a.db.CreateRoutine(in)
		if err != nil {
			a.adt.Error("Couldn't save the routine", err.Error())
			return
		}
		a.routineEditingID = routine.ID
		a.adt.Success("Routine saved", "Next run "+routine.NextRunAt.Local().Format("Jan 2, 3:04 PM"))
	} else {
		if err := a.db.UpdateRoutine(a.routineEditingID, in); err != nil {
			a.adt.Error("Couldn't update the routine", err.Error())
			return
		}
		a.adt.Success("Routine updated", "Your changes are saved.")
	}
	a.refresh()
}

func isPresetRoutineInterval(value string) bool {
	switch value {
	case "5m", "15m", "30m", "1h", "4h", "8h", "12h", "24h":
		return true
	default:
		return false
	}
}

func (a *app) testRoutine(id string) {
	if a.cancelRun != nil {
		a.adt.Warning("A Dot is already running", "Wait for it to finish before running this routine.")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.adt.Info("Routine started", "This one-time run sends a request to OpenAI and may use your plan.")
	prefs := a.prefs
	win := a.win
	go func() {
		err := engine.RunRoutineOnce(ctx, a.db, id, openAIGeneratorWithPreferences(prefs), func() {
			if win != nil {
				win.Update(func() { a.refresh() })
			}
		})
		cancel()
		finish := func() {
			a.cancelRun = nil
			a.refresh()
			if err != nil {
				a.adt.Error("Routine didn't finish", err.Error())
			} else {
				a.adt.Success("Routine completed", "See the Dot conversation for its response.")
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(finish)
	}()
}

func (a *app) startRoutineScheduler() {
	if a.routineSchedulerStarted {
		return
	}
	a.routineSchedulerStarted = true
	if a.routinePreferences == nil {
		a.routinePreferences = &atomic.Pointer[preferences]{}
	}
	prefs := a.prefs
	a.routinePreferences.Store(&prefs)
	win := a.win
	go func() {
		err := engine.RunRoutineLoop(context.Background(), a.db, func() engine.Generator {
			current := a.routinePreferences.Load()
			if current == nil {
				return openAIGeneratorWithPreferences(prefs)
			}
			return openAIGeneratorWithPreferences(*current)
		}, 30*time.Second, func() {
			if win != nil {
				win.Update(func() { a.refresh() })
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			if win != nil {
				win.Update(func() { a.adt.Error("Routine scheduler stopped", err.Error()) })
			}
		}
	}()
}

func (a *app) publishRoutinePreferences() {
	if a.routinePreferences == nil {
		a.routinePreferences = &atomic.Pointer[preferences]{}
	}
	prefs := a.prefs
	a.routinePreferences.Store(&prefs)
}
