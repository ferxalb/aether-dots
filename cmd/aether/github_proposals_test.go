package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

const cliRepoReply = "HTTP/2.0 200 OK\r\nX-Oauth-Scopes: repo\r\nX-Ratelimit-Remaining: 4000\r\n\r\n" +
	`{"id":101,"name":"alpha","full_name":"octocat/alpha","private":true,"archived":false,"has_issues":true,"owner":{"login":"octocat"}}`

type cliEnv struct {
	db      *store.Store
	conv    task.Conversation
	dot     task.Dot
	service *github.Service
	log     string
}

func newCLIEnv(t *testing.T, routes ...ghtest.Route) cliEnv {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conv, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Repo helper"})
	if err != nil {
		t.Fatal(err)
	}
	repo := task.GitHubRepository{ID: 101, Owner: "octocat", Name: "alpha", FullName: "octocat/alpha", HTMLURL: "https://github.com/octocat/alpha", Private: true, DefaultBranch: "main"}
	for _, err := range []error{
		db.SetGitHubConnected(true), db.SetGitHubAccount("octocat"),
		db.ReplaceGitHubRepositories([]task.GitHubRepository{repo}),
		db.SetGitHubRepositorySelection([]int64{101}),
		db.SetConversationGitHubGrants(conv.ID, []int64{101}, true),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	dot, _, err := db.CreateConversationRun(conv.ID, "work")
	if err != nil {
		t.Fatal(err)
	}
	routes = append([]ghtest.Route{{Glob: "/repositories/101", Out: cliRepoReply}}, routes...)
	log := ghtest.Install(t, true, routes...)
	return cliEnv{db: db, conv: conv, dot: dot, service: github.NewService(), log: log}
}

func (e cliEnv) grantAll(t *testing.T) {
	t.Helper()
	if err := grantGitHubWrites(e.db, []string{"--allow-publish", e.conv.ID, "101:comment,issue,pr"}); err != nil {
		t.Fatal(err)
	}
}

func (e cliEnv) propose(t *testing.T, body string) task.GitHubProposal {
	t.Helper()
	p, _, err := e.db.CreateGitHubProposal(e.dot.ID, task.GitHubProposalInput{Kind: task.GitHubIssueComment, RepositoryID: 101, Target: 7, Body: body}, true)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e cliEnv) run(t *testing.T, stdin string, interactive bool, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := githubProposalsCommand(e.db, e.service, strings.NewReader(stdin), &out, interactive, args)
	return out.String(), err
}

func cliPosts(log string) int {
	b, _ := os.ReadFile(log)
	return strings.Count(string(b), "--method POST")
}

var cliCommentRoutes = []ghtest.Route{
	{Glob: "/repos/octocat/alpha/issues/7", Out: `{"number":7,"locked":false}`},
	{Glob: "/repos/octocat/alpha/issues/7/comments", Out: `{"html_url":"https://github.com/octocat/alpha/issues/7#issuecomment-5","user":{"login":"octocat"}}`},
}

func TestParseWriteGrantSpecs(t *testing.T) {
	got, err := parseWriteGrantSpecs([]string{"101:comment,pr", "202:issue_create"})
	if err != nil || len(got) != 2 || got[0].Kinds[1] != task.GitHubPRDraft || got[1].Kinds[0] != task.GitHubIssueCreate {
		t.Fatalf("%#v %v", got, err)
	}
	for _, bad := range []string{"101", "101:", "x:comment", "101:delete", "-5:comment", "0:comment"} {
		if _, err := parseWriteGrantSpecs([]string{bad}); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestWriteGrantNeedsAcknowledgementAndReadGrant(t *testing.T) {
	e := newCLIEnv(t)
	if err := grantGitHubWrites(e.db, []string{e.conv.ID, "101:comment"}); err == nil {
		t.Fatal("write grant was saved without --allow-publish")
	}
	if err := grantGitHubWrites(e.db, []string{"--allow-publish", e.conv.ID, "202:comment"}); err == nil {
		t.Fatal("write grant was saved for a repository the Dot cannot read")
	}
	e.grantAll(t)
	if _, ok := e.db.GitHubWriteGrants(e.dot.ID); !ok {
		t.Fatal("grant was not saved")
	}
}

func TestListAndShowPrintExactTextAndNeutraliseTerminalEscapes(t *testing.T) {
	e := newCLIEnv(t)
	e.grantAll(t)
	p := e.propose(t, "hello \x1b[2J\x1b]0;pwned\x07 cc @maintainer")
	list, err := e.run(t, "", false)
	if err != nil || !strings.Contains(list, p.ID[:12]) || !strings.Contains(list, "pending") || !strings.Contains(list, "Comment on #7") {
		t.Fatalf("list=%q err=%v", list, err)
	}
	show, err := e.run(t, "", false, "show", p.ID[:8])
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(show, 0x1b) || strings.ContainsRune(show, 7) {
		t.Fatalf("raw control characters reached the terminal: %q", show)
	}
	for _, want := range []string{p.ContentHash, "Published as @octocat", "exactly what will be published", "Mentions @maintainer", "outside content"} {
		if !strings.Contains(show, want) {
			t.Fatalf("show lacks %q:\n%s", want, show)
		}
	}
}

func TestProposalIDPrefixIsStrict(t *testing.T) {
	e := newCLIEnv(t)
	e.grantAll(t)
	p := e.propose(t, "x")
	if _, err := e.run(t, "", false, "show", p.ID[:3]); err == nil {
		t.Fatal("a 3-character prefix was accepted")
	}
	if _, err := e.run(t, "", false, "show", "ffffffffff"); err == nil {
		t.Fatal("an unknown id was accepted")
	}
}

func TestApproveRequiresTerminalAndTheTypedHashPrefix(t *testing.T) {
	e := newCLIEnv(t, cliCommentRoutes...)
	e.grantAll(t)
	p := e.propose(t, "Approved text")
	if _, err := e.run(t, p.ContentHash[:8]+"\n", false, "approve", p.ID[:8]); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-interactive approval: %v", err)
	}
	for _, input := range []string{"\n", "", "y\n", "yes\n", strings.ToUpper(p.ContentHash[:7]) + "\n", "deadbeef\n"} {
		if _, err := e.run(t, input, true, "approve", p.ID[:8]); err == nil {
			t.Fatalf("input %q published", input)
		}
	}
	if cliPosts(e.log) != 0 {
		t.Fatal("a rejected confirmation sent a write")
	}
	out, err := e.run(t, strings.ToUpper(p.ContentHash[:8])+"\n", true, "approve", p.ID[:8])
	if err != nil || !strings.Contains(out, "Published: https://github.com/octocat/alpha/issues/7#issuecomment-5") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if cliPosts(e.log) != 1 || ghtest.Stdin(e.log) != `{"body":"Approved text"}` {
		t.Fatalf("posts=%d stdin=%q", cliPosts(e.log), ghtest.Stdin(e.log))
	}
	if _, err := e.run(t, p.ContentHash[:8]+"\n", true, "approve", p.ID[:8]); err == nil {
		t.Fatal("an executed proposal could be approved again")
	}
	if cliPosts(e.log) != 1 {
		t.Fatal("second approval sent again")
	}
}

func TestRejectAndResolveFromCLI(t *testing.T) {
	e := newCLIEnv(t)
	e.grantAll(t)
	a := e.propose(t, "reject me")
	if _, err := e.run(t, "", false, "reject", a.ID[:8]); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.db.GetGitHubProposal(a.ID); got.State != task.GitHubProposalRejected {
		t.Fatalf("state %s", got.State)
	}
	b := e.propose(t, "uncertain one")
	if _, err := e.run(t, "", false, "resolve", b.ID[:8], "--not-created"); err == nil {
		t.Fatal("a pending proposal was resolved")
	}
	if _, err := e.db.BeginGitHubProposalExecution(b.ID, b.ContentHash, "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := e.db.FailGitHubProposal(b.ID, "HTTP 502", true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.run(t, "", false, "resolve", b.ID[:8]); err == nil {
		t.Fatal("resolve without a choice was accepted")
	}
	if _, err := e.run(t, "", false, "resolve", b.ID[:8], "--created", "https://evil.example/x"); err == nil {
		t.Fatal("a non-GitHub link was accepted")
	}
	if _, err := e.run(t, "", false, "resolve", b.ID[:8], "--created", "https://github.com/octocat/alpha/issues/7#issuecomment-9"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.db.GetGitHubProposal(b.ID); got.State != task.GitHubProposalExecuted {
		t.Fatalf("state %s", got.State)
	}
}

func TestTerminalSafe(t *testing.T) {
	if got := terminalSafe("a\x1b[31mb\x07\u009bc\td\ne"); got != "a·[31mb··c\td\ne" {
		t.Fatalf("%q", got)
	}
}
