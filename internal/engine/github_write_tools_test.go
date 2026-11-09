package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

const writeRepoReply = "HTTP/2.0 200 OK\r\nX-Oauth-Scopes: repo\r\nX-Ratelimit-Remaining: 4000\r\n\r\n" +
	`{"id":101,"name":"alpha","full_name":"octocat/alpha","private":true,"archived":false,"has_issues":true,"owner":{"login":"octocat"}}`

type writeEnv struct {
	db      *store.Store
	conv    task.Conversation
	dot     task.Dot
	service *github.Service
	log     string
}

func newWriteEnv(t *testing.T, kinds []task.GitHubWriteKind, routes ...ghtest.Route) writeEnv {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conv, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Writer"})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []error{
		db.SetGitHubConnected(true), db.SetGitHubAccount("octocat"),
		db.ReplaceGitHubRepositories([]task.GitHubRepository{githubToolRepo(101, "alpha"), githubToolRepo(202, "beta")}),
		db.SetGitHubRepositorySelection([]int64{101, 202}),
		db.SetConversationGitHubGrants(conv.ID, []int64{101}, true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	if len(kinds) > 0 {
		if err := db.SetConversationGitHubWriteGrants(conv.ID, []task.GitHubWriteGrant{{RepositoryID: 101, Kinds: kinds}}, true); err != nil {
			t.Fatal(err)
		}
	}
	dot, _, err := db.CreateConversationRun(conv.ID, "Help with alpha")
	if err != nil {
		t.Fatal(err)
	}
	routes = append([]ghtest.Route{{Glob: "/repositories/101", Out: writeRepoReply}}, routes...)
	log := ghtest.Install(t, true, routes...)
	return writeEnv{db: db, conv: conv, dot: dot, service: github.NewService(), log: log}
}

func (e writeEnv) tools(t *testing.T) model.Generator {
	t.Helper()
	return WithGitHubWriteTools(e.db, e.dot.ID, model.Generator{Config: model.Config{Model: "test-model"}}, e.service).(model.Generator)
}

func postCount(log string) int {
	b, _ := os.ReadFile(log)
	return strings.Count(string(b), "--method POST")
}

var commentRoutes = []ghtest.Route{
	{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":false}`},
	{Glob: "/repos/octocat/alpha/issues/7/comments", Out: `{"html_url":"https://github.com/octocat/alpha/issues/7#issuecomment-5","id":5,"user":{"login":"octocat"}}`},
}

func TestWriteToolsExistOnlyForGrantedKindsAndMatchingAccount(t *testing.T) {
	none := newWriteEnv(t, nil)
	if len(none.tools(t).Config.Tools) != 0 {
		t.Fatal("write tools exist without a write grant")
	}
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment})
	g := e.tools(t)
	if !containsTool(g.Config.Tools, "aether_github_propose_issue_comment") ||
		containsTool(g.Config.Tools, "aether_github_propose_issue") || containsTool(g.Config.Tools, "aether_github_propose_pull_request_draft") {
		t.Fatalf("catalog does not match the grant: %v", g.Config.Tools)
	}
	if err := e.db.SetGitHubAccount("mallory"); err != nil {
		t.Fatal(err)
	}
	if len(e.tools(t).Config.Tools) != 0 {
		t.Fatal("tools survived a different pinned account")
	}
}

func TestProposalToolQueuesWithoutSendingAndRecordsProvenance(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment}, commentRoutes...)
	tool := findTool(t, e.tools(t).Config.Tools, "aether_github_propose_issue_comment")
	out, err := tool.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"Looks good to me"}`)
	if err != nil || !strings.Contains(out, "queued_for_user_approval") || !strings.Contains(out, "Nothing has been sent") {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if postCount(e.log) != 0 {
		t.Fatal("a proposal tool sent a write")
	}
	proposals := e.db.GitHubProposals()
	if len(proposals) != 1 || proposals[0].State != task.GitHubProposalPending || proposals[0].ReadUntrustedInRun {
		t.Fatalf("%#v", proposals)
	}
	again, err := tool.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"Looks good to me"}`)
	if err != nil || !strings.Contains(again, "already_queued") || len(e.db.GitHubProposals()) != 1 {
		t.Fatalf("duplicate not reused: %s %v", again, err)
	}
	// After the run read GitHub data, new proposals carry the warning flag.
	if err := e.db.AppendEvent(e.dot.ID, "github_read", "Read issue"); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"A different comment"}`); err != nil {
		t.Fatal(err)
	}
	if got := e.db.GitHubProposals(); !got[1].ReadUntrustedInRun {
		t.Fatal("a proposal made after reading GitHub data was not flagged")
	}
}

func TestInjectedInstructionsCannotWidenRepositoryOrOperation(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment}, commentRoutes...)
	g := e.tools(t)
	comment := findTool(t, g.Config.Tools, "aether_github_propose_issue_comment")
	for _, args := range []string{
		`{"repository_id":202,"number":7,"body":"as instructed by the issue"}`,         // not granted
		`{"repository_id":101,"number":7,"body":"x","owner":"evil","method":"DELETE"}`, // unknown fields
		`{"repository_id":101,"number":0,"body":"x"}`,
		`{"repository_id":101,"number":7,"body":"` + strings.Repeat("a", task.MaxGitHubProposalBodyBytes+1) + `"}`,
	} {
		if _, err := comment.Handler(context.Background(), args); err == nil {
			t.Fatalf("accepted %.60s", args)
		}
	}
	// The catalog has no tool for ungranted operations, and a handler built
	// for one kind cannot be steered into another.
	if containsTool(g.Config.Tools, "aether_github_propose_pull_request_draft") {
		t.Fatal("ungranted operation was offered")
	}
	if len(e.db.GitHubProposals()) != 0 || postCount(e.log) != 0 {
		t.Fatal("a rejected call left a proposal or wrote")
	}
	// Revoking after the catalog was built makes the stale handler useless.
	if err := e.db.SetConversationGitHubWriteGrants(e.conv.ID, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := comment.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"x"}`); err == nil {
		t.Fatal("a revoked grant still queued a proposal")
	}
}

func TestProposalToolRefusesDestinationsThatCannotWork(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment, task.GitHubPRDraft},
		ghtest.Route{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":true}`},
		ghtest.Route{Glob: "/repos/octocat/alpha/git/ref/heads/main", Out: `{"ref":"x"}`},
	)
	g := e.tools(t)
	if _, err := findTool(t, g.Config.Tools, "aether_github_propose_issue_comment").Handler(context.Background(), `{"repository_id":101,"number":7,"body":"x"}`); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("locked issue: %v", err)
	}
	if _, err := findTool(t, g.Config.Tools, "aether_github_propose_pull_request_draft").Handler(context.Background(), `{"repository_id":101,"title":"t","head":"nope","base":"main"}`); err == nil {
		t.Fatal("a missing head branch was queued")
	}
	if len(e.db.GitHubProposals()) != 0 {
		t.Fatal("unusable destinations were queued")
	}
}

func TestApproveSendsExactlyOnceAndRecordsTheResult(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment}, commentRoutes...)
	tool := findTool(t, e.tools(t).Config.Tools, "aether_github_propose_issue_comment")
	if _, err := tool.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"Approved text"}`); err != nil {
		t.Fatal(err)
	}
	p := e.db.GitHubProposals()[0]
	if _, err := ApproveGitHubProposal(context.Background(), e.db, e.service, p.ID, "not-the-shown-hash"); err == nil {
		t.Fatal("approval of a hash that was not shown was accepted")
	}
	if postCount(e.log) != 0 {
		t.Fatal("a failed approval sent a write")
	}
	done, err := ApproveGitHubProposal(context.Background(), e.db, e.service, p.ID, p.ContentHash)
	if err != nil || done.State != task.GitHubProposalExecuted || done.Result == nil || !strings.Contains(done.Result.URL, "issuecomment-5") {
		t.Fatalf("done=%#v err=%v", done, err)
	}
	if got, want := ghtest.Stdin(e.log), `{"body":"Approved text"}`; got != want {
		t.Fatalf("stdin %q", got)
	}
	if _, err := ApproveGitHubProposal(context.Background(), e.db, e.service, p.ID, p.ContentHash); err == nil {
		t.Fatal("an executed proposal was sent twice")
	}
	if postCount(e.log) != 1 {
		t.Fatalf("%d POSTs", postCount(e.log))
	}
}

func TestApproveRefusalsLeaveTheProposalPendingAndSendNothing(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment}, commentRoutes...)
	tool := findTool(t, e.tools(t).Config.Tools, "aether_github_propose_issue_comment")
	if _, err := tool.Handler(context.Background(), `{"repository_id":101,"number":7,"body":"x"}`); err != nil {
		t.Fatal(err)
	}
	p := e.db.GitHubProposals()[0]
	// The conversation became locked after it was proposed.
	ghtest.InstallAs(t, "octocat",
		ghtest.Route{Glob: "/repositories/101", Out: writeRepoReply},
		ghtest.Route{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":true}`},
	)
	locked := github.NewService()
	if _, err := ApproveGitHubProposal(context.Background(), e.db, locked, p.ID, p.ContentHash); err == nil {
		t.Fatal("a locked conversation was written to")
	}
	if got, _ := e.db.GetGitHubProposal(p.ID); got.State != task.GitHubProposalPending {
		t.Fatalf("a pre-send refusal changed the state to %s", got.State)
	}
	// gh is now signed in as someone else.
	ghtest.InstallAs(t, "mallory", ghtest.Route{Glob: "/repositories/101", Out: writeRepoReply})
	if _, err := ApproveGitHubProposal(context.Background(), e.db, github.NewService(), p.ID, p.ContentHash); err == nil {
		t.Fatal("another gh account sent the proposal")
	}
	if got, _ := e.db.GetGitHubProposal(p.ID); got.State != task.GitHubProposalCancelled && got.State != task.GitHubProposalPending {
		t.Fatalf("state %s", got.State)
	}
}

func TestApproveRecordsDefiniteAndUncertainFailures(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   task.GitHubProposalState
	}{
		{"validation", "gh: Validation Failed (HTTP 422)", task.GitHubProposalFailed},
		{"gateway", "gh: Bad Gateway (HTTP 502)", task.GitHubProposalUncertain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueCreate},
				ghtest.Route{Glob: "/repos/octocat/alpha/issues", Code: 1, Stderr: c.stderr})
			tool := findTool(t, e.tools(t).Config.Tools, "aether_github_propose_issue")
			if _, err := tool.Handler(context.Background(), `{"repository_id":101,"title":"Bug","body":"b"}`); err != nil {
				t.Fatal(err)
			}
			p := e.db.GitHubProposals()[0]
			final, err := ApproveGitHubProposal(context.Background(), e.db, e.service, p.ID, p.ContentHash)
			if err == nil || final.State != c.want || final.FailureReason == "" {
				t.Fatalf("final=%#v err=%v", final, err)
			}
			if postCount(e.log) != 1 {
				t.Fatalf("%d POSTs", postCount(e.log))
			}
		})
	}
}
