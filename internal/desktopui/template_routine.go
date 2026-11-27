package desktopui

import (
	"aether-dots/internal/task"

	"github.com/egoist/mygo/ui"
)

// templateRoutinePanel offers an agent made from a template its suggested
// Routine in one click. It appears only while the agent's saved instructions
// still match the template, and creating it is a Pro action like any Routine.
func (a *app) templateRoutinePanel(c *ui.Context, theme *ui.Theme) {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		return
	}
	tmpl, ok := task.TemplateForInstructions(conversation.Instructions)
	if !ok || tmpl.SuggestedRoutinePrompt == "" {
		return
	}
	profilePanel(c, theme, "Daily summary", func() {
		if routine, ok := templateRoutine(a.snapshot, conversation.ID, tmpl); ok {
			state := "Paused"
			if routine.Active {
				state = "Next run " + routine.NextRunAt.Local().Format("Jan 2, 3:04 PM")
			}
			ui.Text(c, routine.Name+" is scheduled · "+state+". Edit or pause it in Settings → Routines.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			return
		}
		ui.Text(c, "Once a day, this agent goes through new and updated issues and lists the three that need you first. It runs while AETHER is open, uses only the repositories granted above, and never posts anything by itself.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		granted := conversationHasGitHubGrants(a.snapshot, conversation.ID)
		if !granted {
			ui.Text(c, "Grant a repository under GitHub access and save the profile first.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
		}
		if !a.features().Pro {
			proNotice(c, theme, "template-routine-pro-notice", "Scheduled summaries are Routines, which are part of AETHER Pro.")
		}
		if appButton(c, theme, "template-routine-add", "Add daily summary", buttonSecondary).Disabled(!granted).Clicked() {
			a.addTemplateRoutine(conversation.ID, tmpl)
		}
	})
}

// templateRoutine finds the Routine created from tmpl for this agent.
func templateRoutine(snapshot task.Database, conversationID string, tmpl task.AgentTemplate) (task.Routine, bool) {
	for _, routine := range snapshot.Routines {
		if routine.ConversationID == conversationID && routine.Prompt == tmpl.SuggestedRoutinePrompt {
			return routine, true
		}
	}
	return task.Routine{}, false
}

func (a *app) addTemplateRoutine(conversationID string, tmpl task.AgentTemplate) {
	if err := a.features().Require("Routines"); err != nil {
		a.proRequired("Daily summaries need AETHER Pro", err)
		return
	}
	if _, ok := templateRoutine(a.snapshot, conversationID, tmpl); ok {
		return
	}
	if !conversationHasGitHubGrants(a.snapshot, conversationID) {
		a.adt.Warning("Grant a repository first", "The summary reads issues, so this agent needs GitHub access to at least one repository.")
		return
	}
	routine, err := a.db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversationID, Name: tmpl.SuggestedRoutineName, Prompt: tmpl.SuggestedRoutinePrompt,
		IntervalSeconds: tmpl.SuggestedRoutineIntervalSeconds, GitHubEnabled: true,
	})
	if err != nil {
		a.adt.Error("Couldn't add the daily summary", err.Error())
		return
	}
	a.adt.Success("Daily summary added", "First run "+routine.NextRunAt.Local().Format("Jan 2, 3:04 PM")+". Results appear in this agent's conversation.")
	a.refresh()
}
