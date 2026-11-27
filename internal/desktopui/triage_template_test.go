package desktopui

import (
	"strings"
	"testing"

	"aether-dots/internal/license"
	"aether-dots/internal/task"

	"github.com/egoist/mygo/ui"
)

func TestWelcomeCreatesTriageAgentAndOpensItsProfile(t *testing.T) {
	a := newMessengerTestApp(t)
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Set up issue triage"); err != nil {
		t.Fatalf("%v: %q", err, tester.Texts())
	}
	conv, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok || conv.Name != task.TriageTemplate.Name || conv.Instructions != task.TriageTemplate.Instructions ||
		conv.Purpose != task.TriageTemplate.Purpose || conv.Color != task.TriageTemplate.Color {
		t.Fatalf("triage agent = %+v", conv)
	}
	if len(conv.GitHubRepositoryIDs) != 0 || conv.GitHubModelDisclosureAccepted {
		t.Fatal("the template granted GitHub access by itself")
	}
	if !a.conversationDetailsOpen {
		t.Fatal("profile did not open for granting a repository")
	}
	tester.Frame()
	if all := strings.Join(tester.Texts(), "\n"); !strings.Contains(all, "Grant it a repository") || !strings.Contains(all, "GitHub access") {
		t.Fatalf("next step not explained: %q", tester.Texts())
	}
}

func TestTriageTemplateCountsTowardTheFreeAgentLimit(t *testing.T) {
	a, _ := newLicenseTestApp(t, nil)
	a.settingsOpen = false
	for i := 0; i < license.FreeAgentLimit; i++ {
		a.startCreator()
	}
	a.startTemplateCreator(task.TriageTemplate)
	if n := len(a.snapshot.Conversations); n != license.FreeAgentLimit {
		t.Fatalf("template bypassed the Free limit: %d agents", n)
	}
}

func grantTriageRepository(t *testing.T, a *app) task.Conversation {
	t.Helper()
	conv, _ := findConversation(a.snapshot, a.selectedConversationID)
	repo := task.GitHubRepository{ID: 101, Owner: "octocat", Name: "alpha", FullName: "octocat/alpha",
		HTMLURL: "https://github.com/octocat/alpha", Private: true, DefaultBranch: "main"}
	for _, err := range []error{
		a.db.SetGitHubConnected(true), a.db.SetGitHubAccount("octocat"),
		a.db.ReplaceGitHubRepositories([]task.GitHubRepository{repo}),
		a.db.SetGitHubRepositorySelection([]int64{101}),
		a.db.SetConversationGitHubGrants(conv.ID, []int64{101}, true),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	a.refresh()
	return conv
}

func TestDailySummaryButtonSchedulesTheTriageRoutine(t *testing.T) {
	a := newMessengerTestApp(t)
	a.startTemplateCreator(task.TriageTemplate)
	tester := ui.NewTester(a.view, 1120, 760)
	tester.Frame()
	if _, ok := tester.Find("Grant a repository under GitHub access and save the profile first."); !ok {
		t.Fatalf("missing grant hint: %q", tester.Texts())
	}
	a.addTemplateRoutine(a.selectedConversationID, task.TriageTemplate)
	if len(a.snapshot.Routines) != 0 {
		t.Fatal("scheduled a summary for an agent without GitHub access")
	}

	conv := grantTriageRepository(t, a)
	tester.Frame()
	clickUIControl(t, tester, "Add daily summary", 1120, 760)
	if len(a.snapshot.Routines) != 1 {
		t.Fatalf("routines = %+v", a.snapshot.Routines)
	}
	r := a.snapshot.Routines[0]
	if r.ConversationID != conv.ID || r.Name != task.TriageTemplate.SuggestedRoutineName || r.Prompt != task.TriageTemplate.SuggestedRoutinePrompt ||
		r.IntervalSeconds != 24*60*60 || !r.GitHubEnabled || !r.Active {
		t.Fatalf("routine = %+v", r)
	}
	tester.Frame()
	if _, ok := tester.Find("Add daily summary"); ok {
		t.Fatal("button still offered after the summary was scheduled")
	}
	if all := strings.Join(tester.Texts(), "\n"); !strings.Contains(all, "Daily triage summary is scheduled") {
		t.Fatalf("scheduled state not shown: %q", tester.Texts())
	}
	a.addTemplateRoutine(conv.ID, task.TriageTemplate)
	if len(a.snapshot.Routines) != 1 {
		t.Fatal("a second click added a duplicate routine")
	}
}

func TestDailySummaryNeedsProAndHidesForEditedAgents(t *testing.T) {
	a, _ := newLicenseTestApp(t, nil)
	a.settingsOpen = false
	a.startTemplateCreator(task.TriageTemplate)
	conv := grantTriageRepository(t, a)
	tester := ui.NewTester(a.view, 1120, 760)
	tester.Frame()
	if all := strings.Join(tester.Texts(), "\n"); !strings.Contains(all, "part of AETHER Pro") {
		t.Fatalf("Pro notice missing on Free: %q", tester.Texts())
	}
	a.addTemplateRoutine(conv.ID, task.TriageTemplate)
	if len(a.snapshot.Routines) != 0 {
		t.Fatal("Free created a routine")
	}

	plain := createTestConversation(t, a, a.selectedSpaceID, "Plain", "No template")
	a.refresh()
	a.selectedConversationID = plain.ID
	tester.Frame()
	if _, ok := tester.Find("Add daily summary"); ok {
		t.Fatal("offered a daily summary to an agent not made from a template")
	}
}
