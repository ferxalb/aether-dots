package desktopui

import (
	"context"
	"errors"
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
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "Skill library").FontSize(15).Bold()
			ui.Text(c, "Save repeatable playbooks. Skills are shared across Dots and only run when a message names them.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth()
		})
		if len(a.snapshot.Skills) == 0 {
			ui.Text(c, "No saved skills yet.").FontSize(12).TextColor(theme.TextMuted)
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
							a.skillMessage = err.Error()
						} else {
							a.skillMessage = "Skill deleted. Saved chat history keeps its original instructions."
							a.skillEditingID, a.skillDeleteID = "", ""
							a.skillDraft = task.Skill{}
							a.refresh()
						}
					}
				} else if ui.Button(c.Key("skill-delete-"+skill.ID), "Remove").Padding(7, 10).Clicked() {
					a.skillDeleteID = skill.ID
				}
			})
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "@skill[Name]").FontSize(11).Bold().TextColor(theme.Accent)
			ui.Text(c, "in a message selects one skill for that turn.").FontSize(11).TextColor(theme.TextMuted)
			ui.Spacer(c)
			if ui.Button(c.Key("skill-new"), "New skill").Padding(7, 10).Clicked() {
				a.skillEditingID, a.skillDeleteID = "", ""
				a.skillDraft = task.Skill{}
				a.skillMessage = ""
			}
		})
		ui.Divider(c)
		ui.Text(c, "Skill details").FontSize(12).Bold()
		ui.Field(c, "Name", func() {
			ui.TextInputBase(c.Key("skill-name"), &a.skillDraft.Name).Placeholder("Weekly account health").FillWidth()
		})
		ui.Field(c, "Description", func() {
			ui.TextInputBase(c.Key("skill-description"), &a.skillDraft.Description).Placeholder("Short summary of the result this playbook produces").FillWidth()
		})
		skillTextArea(c, "skill-when", "When to use", &a.skillDraft.WhenToUse, "Describe the task or conditions that should select this skill.")
		skillTextArea(c, "skill-inputs", "Required inputs", &a.skillDraft.Inputs, "Name the source context and required information. State when data may be unavailable.")
		skillTextArea(c, "skill-steps", "Steps and decision rules", &a.skillDraft.Steps, "Write the workflow in order, including key decision rules.")
		skillTextArea(c, "skill-validation", "Validation", &a.skillDraft.Validation, "Explain how to check the result and its sources.")
		skillTextArea(c, "skill-output", "Expected output", &a.skillDraft.Output, "Describe the format and what the answer should contain.")
		skillTextArea(c, "skill-approval", "Approval boundary", &a.skillDraft.Approval, "State which actions require approval and what the Dot must stop before doing.")
		if a.skillMessage != "" {
			ui.Text(c, a.skillMessage).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c.Key("skill-save"), "Save skill").Padding(9, 13).Clicked() {
				a.saveSkill()
			}
			if a.skillEditingID != "" && ui.Button(c.Key("skill-cancel-edit"), "Cancel edit").Padding(9, 13).Clicked() {
				a.skillEditingID, a.skillDeleteID = "", ""
				a.skillDraft = task.Skill{}
				a.skillMessage = ""
			}
		})
	})
}

func skillTextArea(c *ui.Context, key, label string, value *string, placeholder string) {
	ui.Field(c, label, func() {
		ui.TextAreaBase(c.Key(key), value).MinHeight(52).MaxHeight(140).FillWidth().
			Placeholder(placeholder)
	})
}

func (a *app) editSkill(skill task.Skill) {
	a.skillEditingID = skill.ID
	a.skillDeleteID = ""
	a.skillDraft = skill
	a.skillMessage = ""
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
			a.skillMessage = err.Error()
			return
		}
		a.skillEditingID, a.skillDraft = skill.ID, skill
		a.skillMessage = "Skill saved. Use @skill[" + skill.Name + "] in a message to apply it."
	} else {
		if err := a.db.UpdateSkill(a.skillEditingID, in); err != nil {
			a.skillMessage = err.Error()
			return
		}
		a.skillMessage = "Skill changes saved."
	}
	a.refresh()
}

func (a *app) routineSettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(13).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "Routines").FontSize(15).Bold()
			ui.Text(c, "Run a saved prompt on an interval. Runs use the Dot profile and chat history and appear as normal conversation messages.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth()
		})
		if len(a.snapshot.Routines) == 0 {
			ui.Text(c, "No routines yet.").FontSize(12).TextColor(theme.TextMuted)
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
						ui.Text(c, conversationName+" · every "+task.FormatRoutineInterval(routine.IntervalSeconds)).
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
							a.routineMessage = err.Error()
						} else {
							a.routineMessage = "Routine schedule updated."
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
					if ui.Button(c.Key("routine-test-"+routine.ID), "Test now").Padding(6, 9).Disabled(a.cancelRun != nil).Clicked() {
						a.testRoutine(routine.ID)
					}
					if a.routineDeleteID == routine.ID {
						if ui.Button(c.Key("routine-delete-confirm-"+routine.ID), "Confirm delete").Padding(6, 9).Clicked() {
							if err := a.db.DeleteRoutine(routine.ID); err != nil {
								a.routineMessage = err.Error()
							} else {
								a.routineMessage = "Routine deleted."
								a.routineEditingID, a.routineDeleteID = "", ""
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
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Spacer(c)
			if ui.Button(c.Key("routine-new"), "New routine").Padding(7, 10).Clicked() {
				a.routineEditingID, a.routineDeleteID = "", ""
				a.routineNameDraft, a.routinePromptDraft = "", ""
				a.routineDotDraft, a.routineEveryDraft = a.selectedConversationID, "1h"
				a.routineMessage = ""
			}
		})
		ui.Divider(c)
		ui.Text(c, "Routine details").FontSize(12).Bold()
		ui.Field(c, "Dot", func() { conversationChoice(c, theme, a) })
		ui.Field(c, "Name", func() {
			ui.TextInputBase(c.Key("routine-name"), &a.routineNameDraft).Placeholder("Daily review").FillWidth()
		})
		ui.Field(c, "Run every", func() {
			ui.TextInputBase(c.Key("routine-every"), &a.routineEveryDraft).Placeholder("1h, 8h, 24h").FillWidth()
		})
		ui.Field(c, "Instruction", func() {
			ui.TextAreaBase(c.Key("routine-prompt"), &a.routinePromptDraft).MinHeight(74).MaxHeight(180).FillWidth().
				Placeholder("Review the current conversation context. State missing inputs clearly and return a concise summary.")
		})
		ui.Text(c, "Intervals start after creation or resume (minimum 5m). A test makes a real model call and uses API tokens. The desktop runs schedules while open; use `aether routines serve` in the CLI. Missed intervals are skipped; limits are 200 runs per routine and 300 total per hour.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		if a.routineMessage != "" {
			ui.Text(c, a.routineMessage).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c.Key("routine-save"), "Save routine").Padding(9, 13).Clicked() {
				a.saveRoutine()
			}
			if a.routineEditingID != "" && ui.Button(c.Key("routine-cancel-edit"), "Cancel edit").Padding(9, 13).Clicked() {
				a.routineEditingID, a.routineDeleteID = "", ""
				a.routineNameDraft, a.routinePromptDraft = "", ""
				a.routineEveryDraft, a.routineDotDraft = "1h", a.selectedConversationID
				a.routineMessage = ""
			}
		})
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

func (a *app) editRoutine(routine task.Routine) {
	a.routineEditingID = routine.ID
	a.routineDeleteID = ""
	a.routineNameDraft = routine.Name
	a.routinePromptDraft = routine.Prompt
	a.routineDotDraft = routine.ConversationID
	a.routineEveryDraft = task.FormatRoutineInterval(routine.IntervalSeconds)
	a.routineMessage = ""
}

func (a *app) saveRoutine() {
	interval, err := time.ParseDuration(strings.TrimSpace(a.routineEveryDraft))
	if err != nil {
		a.routineMessage = "Use a Go duration such as 30m, 8h, or 24h."
		return
	}
	in := task.CreateRoutine{
		ConversationID: a.routineDotDraft, Name: a.routineNameDraft,
		Prompt: a.routinePromptDraft, IntervalSeconds: int64(interval / time.Second),
	}
	if a.routineEditingID == "" {
		routine, err := a.db.CreateRoutine(in)
		if err != nil {
			a.routineMessage = err.Error()
			return
		}
		a.routineEditingID = routine.ID
		a.routineMessage = "Routine saved. First run: " + routine.NextRunAt.Local().Format(time.RFC3339)
	} else {
		if err := a.db.UpdateRoutine(a.routineEditingID, in); err != nil {
			a.routineMessage = err.Error()
			return
		}
		a.routineMessage = "Routine changes saved."
	}
	a.refresh()
}

func (a *app) testRoutine(id string) {
	if a.cancelRun != nil {
		a.routineMessage = "Another Dot run is active. Wait for it to finish before testing a routine."
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.message = "Testing routine…"
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
				a.routineMessage = "Routine test failed: " + err.Error()
			} else {
				a.routineMessage = "Routine test completed. See the Dot conversation for its response."
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
				win.Update(func() { a.message = "Routine scheduler stopped: " + err.Error() })
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
