package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/task"
)

func TestGitHubGrantsRequireConnectionSelectionDisclosureAndCurrentSpace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "GitHub reader"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Inspect the selected repository")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("a disconnected Dot received GitHub capabilities")
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 9001, "alpha"), githubRepoFixture(202, 9001, "beta")}); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("a connected account implicitly enabled a Dot")
	}
	if err := db.SetGitHubRepositorySelection([]int64{101, 202}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, false); err == nil {
		t.Fatal("a repository grant did not require the model disclosure")
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	granted, ok := db.GitHubToolRepositories(dot.ID)
	if !ok || len(granted) != 1 || granted[0].ID != 101 {
		t.Fatalf("Dot received the wrong repository grant: %#v", granted)
	}
	if _, ok := db.GitHubToolRepository(dot.ID, 202); ok {
		t.Fatal("repository B bypassed the repository A grant")
	}
	if err := db.SetGitHubRepositorySelection([]int64{202}); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("removing a repository selection left a stale Dot grant")
	}

	if err := db.SetGitHubRepositorySelection([]int64{101}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	otherSpace, err := db.CreateSpace("Private workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateConversationProfileAndGitHub(conversation.ID, otherSpace.ID, conversation.Name, conversation.Icon, conversation.Color,
		conversation.Purpose, conversation.Instructions, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("moving a Dot to another Space preserved its GitHub grant")
	}

	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(db.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ghu_", "ghr_", "access_token", "refresh_token", "device_code"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("GitHub credentials leaked into task state (%q): %s", forbidden, serialized)
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopenedConversation, ok := findConversationForTest(reopened.Snapshot(), conversation.ID)
	if !ok || !reopenedConversation.GitHubModelDisclosureAccepted || len(reopenedConversation.GitHubRepositoryIDs) != 1 {
		t.Fatalf("approved selection did not survive reopening: %#v", reopenedConversation)
	}
	if err := db.DisconnectGitHub(); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("disconnect left a GitHub capability active")
	}
	if len(db.Snapshot().GitHubRepositories) != 0 || db.Snapshot().GitHubConnected {
		t.Fatal("disconnect did not clear locally selected GitHub resources")
	}
}

func TestRoutineNeedsSeparateGitHubOptInAndLosesItWhenDotGrantIsRemoved(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Scheduled reader"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 9001, "alpha")}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Morning review", Prompt: "Check the repository.",
		IntervalSeconds: task.MinRoutineIntervalSeconds, GitHubEnabled: true,
	}); err == nil {
		t.Fatal("routine without a Dot grant was authorized for GitHub")
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Morning review", Prompt: "Check the repository.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, dot, err := db.StartRoutineNow(routine.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok {
		t.Fatal("the Dot grant alone implicitly enabled a Routine")
	}
	if err := db.SetRoutineGitHubEnabled(routine.ID, true); err != nil {
		t.Fatal(err)
	}
	if repositories, ok := db.GitHubToolRepositories(dot.ID); !ok || len(repositories) != 1 || repositories[0].ID != 101 {
		t.Fatalf("explicitly enabled Routine received no grant: %#v", repositories)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.GitHubToolRepositories(dot.ID); ok || db.Snapshot().Routines[0].GitHubEnabled {
		t.Fatal("removing the Dot grant left its Routine capability active")
	}
}

func TestGitHubDisconnectRejectsLateRepositoryRefresh(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.DisconnectGitHub(); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 9001, "alpha")}); err == nil {
		t.Fatal("late refresh repopulated repositories after disconnect")
	}
	if repositories := db.GitHubRepositories(); len(repositories) != 0 {
		t.Fatalf("disconnected GitHub state retained repository metadata: %#v", repositories)
	}
}

func TestSchemaV4MigrationPreservesDotsAndRoutinesWithoutGitHubGrants(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Existing Dot"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Existing request")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Done, "Existing response completed."); err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Existing routine", Prompt: "Keep existing behavior.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 9001, "alpha")}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRoutineGitHubEnabled(routine.ID, true); err != nil {
		t.Fatal(err)
	}
	legacy := legacyDir(t, db, func(raw map[string]any) { raw["schema_version"] = float64(4) })
	migrated, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := migrated.Snapshot()
	if snapshot.SchemaVersion != task.SchemaVersion || len(snapshot.Conversations) != 1 || len(snapshot.Dots) != 1 || len(snapshot.Routines) != 1 {
		t.Fatalf("migration lost existing product data: %#v", snapshot)
	}
	if snapshot.GitHubConnected || len(snapshot.GitHubRepositories) != 0 || len(snapshot.Conversations[0].GitHubRepositoryIDs) != 0 || snapshot.Routines[0].GitHubEnabled {
		t.Fatalf("v4 migration left GitHub authorization enabled: %#v", snapshot)
	}
	if snapshot.Dots[0].ID != dot.ID || snapshot.Dots[0].Status != task.Done || verifyEvents(snapshot) != nil {
		t.Fatalf("migrated task/event history was not preserved: %#v", snapshot.Dots)
	}
}

func githubRepoFixture(id, installationID int64, name string) task.GitHubRepository {
	return task.GitHubRepository{
		ID: id, Owner: "octocat", Name: name,
		FullName: "octocat/" + name, HTMLURL: "https://github.com/octocat/" + name,
		Private: true, DefaultBranch: "main",
	}
}

func TestSwitchingGitHubAccountDropsPreviousAccountsGrants(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Repo Dot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubAccount("octocat"); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubRepoFixture(101, 0, "alpha")}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	// Re-pinning the same account (any case) keeps everything.
	if err := db.SetGitHubAccount("OctoCat"); err != nil {
		t.Fatal(err)
	}
	if len(db.GitHubRepositories()) != 1 {
		t.Fatal("re-confirming the same account dropped repositories")
	}
	if err := db.SetGitHubAccount("someone-else"); err != nil {
		t.Fatal(err)
	}
	snapshot := db.Snapshot()
	if len(snapshot.GitHubRepositories) != 0 || len(snapshot.Conversations[0].GitHubRepositoryIDs) != 0 || snapshot.Conversations[0].GitHubModelDisclosureAccepted {
		t.Fatalf("grants from the previous GitHub account survived the switch: %#v", snapshot)
	}
	if snapshot.GitHubAccount != "someone-else" {
		t.Fatalf("account=%q", snapshot.GitHubAccount)
	}
	if err := db.SetGitHubAccount("bad name/../x"); err == nil {
		t.Fatal("invalid login accepted")
	}
	if err := db.DisconnectGitHub(); err != nil || db.GitHubAccount() != "" {
		t.Fatalf("disconnect left the account pinned: %q %v", db.GitHubAccount(), err)
	}
}
