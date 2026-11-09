package store

import (
	"strings"
	"sync"
	"testing"
	"time"

	"aether-dots/internal/task"
)

type writeFixture struct {
	db           *Store
	conversation task.Conversation
	dot          task.Dot
}

// newWriteFixture connects GitHub, selects repository 101, grants the Dot
// read access, and (optionally) the given write kinds.
func newWriteFixture(t *testing.T, kinds ...task.GitHubWriteKind) writeFixture {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conv, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Writer"})
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SetGitHubConnected(true))
	must(db.SetGitHubAccount("octocat"))
	must(db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 0, "alpha"), githubRepoFixture(202, 0, "beta")}))
	must(db.SetGitHubRepositorySelection([]int64{101, 202}))
	must(db.SetConversationGitHubGrants(conv.ID, []int64{101}, true))
	if len(kinds) > 0 {
		must(db.SetConversationGitHubWriteGrants(conv.ID, []task.GitHubWriteGrant{{RepositoryID: 101, Kinds: kinds}}, true))
	}
	dot, _, err := db.CreateConversationRun(conv.ID, "Work on alpha")
	must(err)
	return writeFixture{db: db, conversation: conv, dot: dot}
}

func comment(body string) task.GitHubProposalInput {
	return task.GitHubProposalInput{Kind: task.GitHubIssueComment, RepositoryID: 101, Target: 7, Body: body}
}

func TestWriteGrantNeedsReadGrantDisclosureAndKnownKinds(t *testing.T) {
	f := newWriteFixture(t)
	grant := func(repo int64, ok bool, kinds ...task.GitHubWriteKind) error {
		return f.db.SetConversationGitHubWriteGrants(f.conversation.ID, []task.GitHubWriteGrant{{RepositoryID: repo, Kinds: kinds}}, ok)
	}
	if err := grant(101, false, task.GitHubIssueComment); err == nil {
		t.Fatal("write grant did not require the disclosure")
	}
	if err := grant(202, true, task.GitHubIssueComment); err == nil {
		t.Fatal("write grant was accepted without a read grant")
	}
	if err := grant(101, true, "delete_repo"); err == nil {
		t.Fatal("unknown operation was accepted")
	}
	if _, ok := f.db.GitHubWriteGrants(f.dot.ID); ok {
		t.Fatal("rejected grants still enabled writes")
	}
	if err := grant(101, true, task.GitHubIssueComment, task.GitHubIssueComment); err != nil {
		t.Fatal(err)
	}
	grants, ok := f.db.GitHubWriteGrants(f.dot.ID)
	if !ok || len(grants) != 1 || len(grants[0].Kinds) != 1 {
		t.Fatalf("unexpected grants %#v", grants)
	}
}

func TestProposalRequiresGrantedKindAndInteractiveRun(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, task.GitHubProposalInput{Kind: task.GitHubIssueCreate, RepositoryID: 101, Title: "t"}, false); err == nil {
		t.Fatal("an ungranted operation was queued")
	}
	other := comment("hi")
	other.RepositoryID = 202
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, other, false); err == nil {
		t.Fatal("an ungranted repository was queued")
	}
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, comment("hi"), false); err != nil {
		t.Fatal(err)
	}
	// Delegated and Routine runs never receive write tools.
	if err := f.db.change(func(db *task.Database) error {
		db.Dots[dotIndex(db, f.dot.ID)].DelegatedByDotID = "someone"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.db.GitHubWriteGrants(f.dot.ID); ok {
		t.Fatal("a delegated run received write grants")
	}
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, comment("again"), false); err == nil {
		t.Fatal("a delegated run queued a write")
	}
}

func TestProposalValidationAndDeduplication(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment, task.GitHubIssueCreate, task.GitHubPRDraft)
	bad := []task.GitHubProposalInput{
		{Kind: task.GitHubIssueComment, RepositoryID: 101, Body: "no target"},
		{Kind: task.GitHubIssueComment, RepositoryID: 101, Target: 1, Body: " "},
		{Kind: task.GitHubIssueComment, RepositoryID: 101, Target: 1, Body: strings.Repeat("x", task.MaxGitHubProposalBodyBytes+1)},
		{Kind: task.GitHubIssueCreate, RepositoryID: 101, Title: "multi\nline"},
		{Kind: task.GitHubPRDraft, RepositoryID: 101, Title: "t", Head: "-flag", Base: "main"},
		{Kind: task.GitHubPRDraft, RepositoryID: 101, Title: "t", Head: "a..b", Base: "main"},
		{Kind: task.GitHubPRDraft, RepositoryID: 101, Title: "t", Head: "main", Base: "main"},
		{Kind: task.GitHubPRDraft, RepositoryID: 101, Title: "t", Head: "feat", Base: "main", Target: 3},
	}
	for i, in := range bad {
		if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, in, false); err == nil {
			t.Fatalf("invalid proposal %d was accepted", i)
		}
	}
	first, existing, err := f.db.CreateGitHubProposal(f.dot.ID, comment("same"), true)
	if err != nil || existing || first.State != task.GitHubProposalPending || !first.ReadUntrustedInRun {
		t.Fatalf("first=%#v existing=%v err=%v", first, existing, err)
	}
	if first.ExpiresAt.Sub(first.CreatedAt) != task.GitHubProposalTTL || first.Account != "octocat" || first.RepositoryName != "octocat/alpha" {
		t.Fatalf("snapshot fields wrong: %#v", first)
	}
	again, existing, err := f.db.CreateGitHubProposal(f.dot.ID, comment("same"), false)
	if err != nil || !existing || again.ID != first.ID {
		t.Fatalf("duplicate was not reused: %#v %v %v", again, existing, err)
	}
}

func TestProposalLimitsPerRunAndPerDot(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	for i := 0; i < task.MaxGitHubPendingProposalsPerRun; i++ {
		in := comment("c")
		in.Target = int64(i + 1)
		if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, in, false); err != nil {
			t.Fatal(err)
		}
	}
	in := comment("overflow")
	in.Target = 99
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, in, false); err == nil {
		t.Fatal("per-run limit not enforced")
	}
	// Other runs of the same Dot are limited by the per-Dot cap.
	created := task.MaxGitHubPendingProposalsPerRun
	finish := func(id string) {
		if err := f.db.change(func(db *task.Database) error {
			db.Dots[dotIndex(db, id)].Status = task.Done
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	finish(f.dot.ID)
	for created < task.MaxGitHubPendingProposalsPerDot {
		run, _, err := f.db.CreateConversationRun(f.conversation.ID, "another run")
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < task.MaxGitHubPendingProposalsPerRun && created < task.MaxGitHubPendingProposalsPerDot; j++ {
			in := comment("c")
			in.Target = int64(100 + created)
			if _, _, err := f.db.CreateGitHubProposal(run.ID, in, false); err != nil {
				t.Fatal(err)
			}
			created++
		}
		finish(run.ID)
	}
	run, _, _ := f.db.CreateConversationRun(f.conversation.ID, "last run")
	in = comment("one too many")
	in.Target = 500
	if _, _, err := f.db.CreateGitHubProposal(run.ID, in, false); err == nil {
		t.Fatal("per-Dot limit not enforced")
	}
}

func TestApprovalIsBoundToTheShownHashAndEditsRehash(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	p, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("original"), false)
	edited, err := f.db.EditGitHubProposal(p.ID, "", "edited text")
	if err != nil || edited.ContentHash == p.ContentHash || edited.Body != "edited text" {
		t.Fatalf("edit did not rehash: %#v %v", edited, err)
	}
	if _, err := f.db.EditGitHubProposal(p.ID, "title on a comment", "x"); err == nil {
		t.Fatal("a comment accepted a title")
	}
	if _, err := f.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err == nil {
		t.Fatal("approval of the pre-edit hash was accepted")
	}
	if _, err := f.db.BeginGitHubProposalExecution(p.ID, edited.ContentHash, "someone-else"); err == nil {
		t.Fatal("a different gh account could send the proposal")
	}
	// Tampering with stored content without rehashing must be caught.
	if err := f.db.change(func(db *task.Database) error { db.GitHubProposals[0].Body = "tampered"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.BeginGitHubProposalExecution(p.ID, edited.ContentHash, "OctoCat"); err == nil {
		t.Fatal("tampered content was sent")
	}
}

func TestStateMachineAndEventsNeverContainTheBody(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	const secret = "SECRET-BODY-TEXT"
	p, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment(secret), false)
	claimed, err := f.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "OctoCat")
	if err != nil || claimed.State != task.GitHubProposalExecuting {
		t.Fatalf("%#v %v", claimed, err)
	}
	if _, err := f.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err == nil {
		t.Fatal("a proposal was claimed twice")
	}
	if err := f.db.RejectGitHubProposal(p.ID); err == nil {
		t.Fatal("an executing proposal could be rejected")
	}
	if err := f.db.CompleteGitHubProposal(p.ID, task.GitHubProposalResult{URL: "https://evil.example/x"}); err == nil {
		t.Fatal("a non-GitHub result link was accepted")
	}
	if err := f.db.FailGitHubProposal(p.ID, "timeout", true); err != nil {
		t.Fatal(err)
	}
	got, _ := f.db.GetGitHubProposal(p.ID)
	if got.State != task.GitHubProposalUncertain {
		t.Fatalf("state %s", got.State)
	}
	if err := f.db.CompleteGitHubProposal(p.ID, task.GitHubProposalResult{URL: "https://github.com/octocat/alpha/issues/7#issuecomment-1"}); err != nil {
		t.Fatal(err)
	}
	got, _ = f.db.GetGitHubProposal(p.ID)
	if got.State != task.GitHubProposalExecuted || got.Result == nil {
		t.Fatalf("not executed: %#v", got)
	}
	if err := f.db.FailGitHubProposal(p.ID, "late", false); err == nil {
		t.Fatal("a finished proposal changed state")
	}
	dot, _ := f.db.GetDot(f.dot.ID)
	var kinds []string
	for _, e := range dot.Events {
		if strings.Contains(e.Message, secret) {
			t.Fatalf("event leaked the body: %q", e.Message)
		}
		if strings.HasPrefix(e.Type, "github_write_") {
			kinds = append(kinds, e.Type)
		}
	}
	want := "github_write_proposed github_write_approved github_write_uncertain github_write_executed"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("events %v", kinds)
	}
}

func TestTwoProcessesCannotBothSendOneProposal(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	p, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("race"), false)
	second, err := Open(f.db.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var wg sync.WaitGroup
	wins := make(chan string, 2)
	for name, s := range map[string]*Store{"desktop": f.db, "cli": second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err == nil {
				wins <- name
			}
		}()
	}
	wg.Wait()
	close(wins)
	if n := len(wins); n != 1 {
		t.Fatalf("%d processes claimed the proposal", n)
	}
}

func TestRevokingOrChangingAccountCancelsPendingProposals(t *testing.T) {
	cases := map[string]func(f writeFixture) error{
		"write grant removed": func(f writeFixture) error {
			return f.db.SetConversationGitHubWriteGrants(f.conversation.ID, nil, false)
		},
		"read grant removed": func(f writeFixture) error {
			return f.db.SetConversationGitHubGrants(f.conversation.ID, nil, false)
		},
		"repository deselected": func(f writeFixture) error { return f.db.SetGitHubRepositorySelection([]int64{202}) },
		"disconnected":          func(f writeFixture) error { return f.db.DisconnectGitHub() },
		"account switched":      func(f writeFixture) error { return f.db.SetGitHubAccount("mallory") },
	}
	for name, revoke := range cases {
		t.Run(name, func(t *testing.T) {
			f := newWriteFixture(t, task.GitHubIssueComment)
			p, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("pending"), false)
			if err := revoke(f); err != nil {
				t.Fatal(err)
			}
			got, _ := f.db.GetGitHubProposal(p.ID)
			if got.State != task.GitHubProposalCancelled {
				t.Fatalf("state %s, want cancelled", got.State)
			}
			if _, err := f.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err == nil {
				t.Fatal("a cancelled proposal could be sent")
			}
		})
	}
}

func TestProposalsExpireAfterTTLAndStaleExecutionBecomesUncertain(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	a, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("old"), false)
	b, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("stuck"), false)
	if _, err := f.db.BeginGitHubProposalExecution(b.ID, b.ContentHash, "octocat"); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-task.GitHubProposalTTL - time.Minute)
	if err := f.db.change(func(db *task.Database) error {
		db.GitHubProposals[0].ExpiresAt = past
		db.GitHubProposals[1].ExecutingSince = time.Now().Add(-task.GitHubProposalExecutionStaleTime - time.Second)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	gotA, _ := f.db.GetGitHubProposal(a.ID)
	gotB, _ := f.db.GetGitHubProposal(b.ID)
	if gotA.State != task.GitHubProposalExpired || gotB.State != task.GitHubProposalUncertain {
		t.Fatalf("a=%s b=%s", gotA.State, gotB.State)
	}
	if _, err := f.db.BeginGitHubProposalExecution(a.ID, a.ContentHash, "octocat"); err == nil {
		t.Fatal("an expired proposal could be sent")
	}
}

func TestProposalsPersistAndCloneIndependently(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	p, _, _ := f.db.CreateGitHubProposal(f.dot.ID, comment("persist"), false)
	if _, err := f.db.BeginGitHubProposalExecution(p.ID, p.ContentHash, "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.CompleteGitHubProposal(p.ID, task.GitHubProposalResult{URL: "https://github.com/octocat/alpha/issues/7#c", Number: 1}); err != nil {
		t.Fatal(err)
	}
	snap := f.db.Snapshot()
	snap.GitHubProposals[0].Result.URL = "mutated"
	snap.Conversations[0].GitHubWriteGrants[0].Kinds[0] = "mutated"
	reopened, err := Open(f.db.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, ok := reopened.GetGitHubProposal(p.ID)
	if !ok || got.State != task.GitHubProposalExecuted || got.Result == nil || got.Result.URL != "https://github.com/octocat/alpha/issues/7#c" {
		t.Fatalf("not persisted or aliased: %#v", got)
	}
	if _, ok := reopened.GitHubWriteGrants(f.dot.ID); !ok {
		t.Fatal("write grant lost or aliased")
	}
}

func TestPrunesOldestFinishedProposalAtCapacity(t *testing.T) {
	f := newWriteFixture(t, task.GitHubIssueComment)
	if err := f.db.change(func(db *task.Database) error {
		for i := 0; i < task.MaxGitHubProposals; i++ {
			db.GitHubProposals = append(db.GitHubProposals, task.GitHubProposal{
				ID: "old" + string(rune('A'+i%26)) + strings.Repeat("x", i), State: task.GitHubProposalRejected,
			})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.db.CreateGitHubProposal(f.dot.ID, comment("fits after pruning"), false); err != nil {
		t.Fatal(err)
	}
	if n := len(f.db.GitHubProposals()); n != task.MaxGitHubProposals {
		t.Fatalf("tray holds %d proposals", n)
	}
}
