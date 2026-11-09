package desktopui

import (
	"os"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

const approvalsRepoReply = "HTTP/2.0 200 OK\r\nX-Oauth-Scopes: repo\r\nX-Ratelimit-Remaining: 4000\r\n\r\n" +
	`{"id":101,"name":"alpha","full_name":"octocat/alpha","private":true,"archived":false,"has_issues":true,"owner":{"login":"octocat"}}`

type approvalsEnv struct {
	a    *app
	conv task.Conversation
	dot  task.Dot
	log  string
}

func newApprovalsEnv(t *testing.T, grantWrites bool, routes ...ghtest.Route) approvalsEnv {
	t.Helper()
	a := newMessengerTestApp(t)
	conv := createTestConversation(t, a, a.selectedSpaceID, "Repo helper", "Help with alpha")
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
	if grantWrites {
		kinds := []task.GitHubWriteKind{task.GitHubIssueComment, task.GitHubIssueCreate, task.GitHubPRDraft}
		if err := a.db.SetConversationGitHubWriteGrants(conv.ID, []task.GitHubWriteGrant{{RepositoryID: 101, Kinds: kinds}}, true); err != nil {
			t.Fatal(err)
		}
	}
	dot, _, err := a.db.CreateConversationRun(conv.ID, "work")
	if err != nil {
		t.Fatal(err)
	}
	routes = append([]ghtest.Route{{Glob: "/repositories/101", Out: approvalsRepoReply}}, routes...)
	log := ghtest.Install(t, true, routes...)
	a.githubService = github.NewService()
	a.refresh()
	return approvalsEnv{a: a, conv: conv, dot: dot, log: log}
}

func (e approvalsEnv) propose(t *testing.T, in task.GitHubProposalInput, untrusted bool) task.GitHubProposal {
	t.Helper()
	p, _, err := e.a.db.CreateGitHubProposal(e.dot.ID, in, untrusted)
	if err != nil {
		t.Fatal(err)
	}
	e.a.refresh()
	return p
}

func commentInput(body string) task.GitHubProposalInput {
	return task.GitHubProposalInput{Kind: task.GitHubIssueComment, RepositoryID: 101, Target: 7, Body: body}
}

func posts(log string) int {
	b, _ := os.ReadFile(log)
	return strings.Count(string(b), "--method POST")
}

func TestApprovalsTrayShowsFullTextWarningsAndSidebarBadge(t *testing.T) {
	e := newApprovalsEnv(t, true)
	p := e.propose(t, commentInput("Please ping @maintainer.\nSecond line of the exact text."), true)
	tester := ui.NewTester(e.a.view, 1120, 760)
	if !tester.HasText("GitHub approvals") {
		t.Fatalf("sidebar entry missing: %q", tester.Texts())
	}
	if err := tester.Click("GitHub approvals"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Dots can only propose. Nothing reaches GitHub until you approve the exact text below. Approved items are published as @octocat.",
		"Comment on #7 · octocat/alpha", "Waiting", "Approve and send", "Edit", "Reject",
		"Content " + p.ContentHash[:12],
		"Mentions @maintainer: GitHub will notify them.",
	} {
		if !tester.HasText(want) {
			t.Fatalf("tray is missing %q: %q", want, tester.Texts())
		}
	}
	joined := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(joined, "Second line of the exact text.") || !strings.Contains(joined, "outside content") {
		t.Fatalf("body or provenance warning not shown: %q", joined)
	}
	if posts(e.log) != 0 {
		t.Fatal("opening the tray sent something")
	}
}

func TestApproveFromTraySendsOnceAndShowsThePublishedLink(t *testing.T) {
	e := newApprovalsEnv(t, true,
		ghtest.Route{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":false}`},
		ghtest.Route{Glob: "/repos/octocat/alpha/issues/7/comments", Out: `{"html_url":"https://github.com/octocat/alpha/issues/7#issuecomment-5","user":{"login":"octocat"}}`},
	)
	p := e.propose(t, commentInput("Approved text"), false)
	e.a.openApprovals()
	tester := ui.NewTester(e.a.view, 1120, 760)
	if err := tester.Click("Approve and send"); err != nil {
		t.Fatal(err)
	}
	got, _ := e.a.db.GetGitHubProposal(p.ID)
	if got.State != task.GitHubProposalExecuted || posts(e.log) != 1 || ghtest.Stdin(e.log) != `{"body":"Approved text"}` {
		t.Fatalf("state=%s posts=%d stdin=%q", got.State, posts(e.log), ghtest.Stdin(e.log))
	}
	tester = ui.NewTester(e.a.view, 1120, 760)
	if !tester.HasText("Published") || tester.HasText("Approve and send") {
		t.Fatalf("tray did not move the item to history: %q", tester.Texts())
	}
	if !strings.Contains(strings.Join(tester.Texts(), "\n"), "issuecomment-5") {
		t.Fatalf("published link missing: %q", tester.Texts())
	}
}

func TestApproveRefusalLeavesProposalPendingWithAnExplanation(t *testing.T) {
	e := newApprovalsEnv(t, true, ghtest.Route{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":false}`})
	p := e.propose(t, commentInput("will be refused"), false)
	// The conversation is locked by the time the person approves.
	ghtest.Install(t, true,
		ghtest.Route{Glob: "/repositories/101", Out: approvalsRepoReply},
		ghtest.Route{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":true}`})
	e.a.githubService = github.NewService()
	e.a.openApprovals()
	tester := ui.NewTester(e.a.view, 1120, 760)
	if err := tester.Click("Approve and send"); err != nil {
		t.Fatal(err)
	}
	got, _ := e.a.db.GetGitHubProposal(p.ID)
	if got.State != task.GitHubProposalPending {
		t.Fatalf("state %s", got.State)
	}
	tester = ui.NewTester(e.a.view, 1120, 760)
	if !strings.Contains(strings.Join(tester.Texts(), "\n"), "Nothing was sent.") || !tester.HasText("Approve and send") {
		t.Fatalf("refusal not explained or retry unavailable: %q", tester.Texts())
	}
}

func TestRejectAndEditFromTray(t *testing.T) {
	e := newApprovalsEnv(t, true)
	p := e.propose(t, commentInput("first draft"), false)
	e.a.openApprovals()
	tester := ui.NewTester(e.a.view, 1120, 760)
	if err := tester.Click("Edit"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Save changes") || e.a.approvals.bodyDraft != "first draft" {
		t.Fatalf("edit mode did not open with the current text: %q", tester.Texts())
	}
	e.a.approvals.bodyDraft = "second draft"
	if err := tester.Click("Save changes"); err != nil {
		t.Fatal(err)
	}
	edited, _ := e.a.db.GetGitHubProposal(p.ID)
	if edited.Body != "second draft" || edited.ContentHash == p.ContentHash || e.a.approvals.editingID != "" {
		t.Fatalf("edit not applied/rehashed: %#v", edited)
	}
	tester = ui.NewTester(e.a.view, 1120, 760)
	if !tester.HasText("Content " + edited.ContentHash[:12]) {
		t.Fatalf("tray shows a stale hash: %q", tester.Texts())
	}
	if err := tester.Click("Reject"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.a.db.GetGitHubProposal(p.ID); got.State != task.GitHubProposalRejected || posts(e.log) != 0 {
		t.Fatalf("state %s posts %d", got.State, posts(e.log))
	}
}

func TestExpiredProposalCannotBeApprovedFromTheTray(t *testing.T) {
	e := newApprovalsEnv(t, true)
	p := e.propose(t, commentInput("too late"), false)
	e.a.snapshot.GitHubProposals[0].ExpiresAt = time.Now().Add(-time.Minute)
	if proposalDisplayState(e.a.snapshot.GitHubProposals[0], time.Now()) != task.GitHubProposalExpired || e.a.pendingApprovalCount() != 0 {
		t.Fatal("overdue proposal still counts as waiting")
	}
	e.a.approvals.open = true
	tester := ui.NewTester(e.a.view, 1120, 760)
	if tester.HasText("Approve and send") || !tester.HasText("Expired") {
		t.Fatalf("expired proposal is actionable: %q", tester.Texts())
	}
	_ = p
}

func TestUncertainProposalIsResolvedByThePerson(t *testing.T) {
	e := newApprovalsEnv(t, true)
	p := e.propose(t, commentInput("maybe sent"), false)
	if _, err := e.a.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := e.a.db.FailGitHubProposal(p.ID, "GitHub answered HTTP 502", true); err != nil {
		t.Fatal(err)
	}
	e.a.refresh()
	e.a.approvals.open = true
	tester := ui.NewTester(e.a.view, 1120, 760)
	for _, want := range []string{"Needs checking", "Check GitHub", "GitHub answered HTTP 502", "It was created", "It was not created"} {
		if !tester.HasText(want) {
			t.Fatalf("missing %q: %q", want, tester.Texts())
		}
	}
	if tester.HasText("Approve and send") {
		t.Fatal("an uncertain proposal offered a second send")
	}
	if err := tester.Click("It was not created"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.a.db.GetGitHubProposal(p.ID); got.State != task.GitHubProposalFailed {
		t.Fatalf("state %s", got.State)
	}
}

func TestProfileWriteGrantPanelRequiresReadGrantAndSavesIndependently(t *testing.T) {
	e := newApprovalsEnv(t, false)
	e.a.selectConversation(e.conv.ID)
	e.a.openConversationProfile()
	tester := ui.NewTester(e.a.view, 1120, 760)
	clickUIControl(t, tester, "Open issues", 1120, 760)
	clickUIControl(t, tester, "I understand approved proposals are published under my GitHub account", 1120, 760)
	clickUIControl(t, tester, "Save write permissions", 1120, 760)
	conv, _ := findConversation(e.a.db.Snapshot(), e.conv.ID)
	if len(conv.GitHubWriteGrants) != 1 || len(conv.GitHubWriteGrants[0].Kinds) != 1 || conv.GitHubWriteGrants[0].Kinds[0] != task.GitHubIssueCreate || !conv.GitHubWriteDisclosureAccepted {
		t.Fatalf("grant not saved as selected: %#v", conv.GitHubWriteGrants)
	}
}

func TestWriteGrantPanelExplainsMissingReadGrant(t *testing.T) {
	a := newMessengerTestApp(t)
	conv := createTestConversation(t, a, a.selectedSpaceID, "No GitHub", "x")
	a.selectConversation(conv.ID)
	a.openConversationProfile()
	tester := ui.NewTester(a.view, 1120, 760)
	clickUIControl(t, tester, "GitHub write proposals", 1120, 760) // scroll the panel into view
	if !tester.HasText("Save a GitHub read grant for at least one repository above first. Write proposals only exist for repositories this Dot can already read.") {
		t.Fatalf("%q", tester.Texts())
	}
	if tester.HasText("Save write permissions") {
		t.Fatal("write grants were offered without a read grant")
	}
}
