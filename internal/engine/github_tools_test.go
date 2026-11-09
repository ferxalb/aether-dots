package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func TestGitHubToolsArePerDotReadOnlyAndRecheckGrantsAtInvocation(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	parentConversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "Repository owner"})
	if err != nil {
		t.Fatal(err)
	}
	childConversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "Specialist"})
	if err != nil {
		t.Fatal(err)
	}
	parentDot, _, err := db.CreateConversationRun(parentConversation.ID, "Review my selected repository")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(parentDot.ID, task.Running, "test run started"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(parentDot.ID, task.Done, "test run completed"); err != nil {
		t.Fatal(err)
	}
	childDot, _, err := db.CreateConversationRun(childConversation.ID, "Review my own context")
	if err != nil {
		t.Fatal(err)
	}
	base := model.Generator{Config: model.Config{Model: "test-model"}}
	service, _ := testGitHubToolService(t)

	withoutConnection := WithGitHubTools(db, parentDot.ID, base, service).(model.Generator)
	if containsTool(withoutConnection.Config.Tools, "aether_github_get_repository") {
		t.Fatal("GitHub tools were registered before the user connected")
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubToolRepo(101, "alpha"), githubToolRepo(202, "beta")}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101, 202}); err != nil {
		t.Fatal(err)
	}
	withoutDotGrant := WithGitHubTools(db, parentDot.ID, base, service).(model.Generator)
	if containsTool(withoutDotGrant.Config.Tools, "aether_github_get_repository") {
		t.Fatal("connected but ungranted Dot received GitHub tools")
	}
	if err := db.SetConversationGitHubGrants(parentConversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	parentWithGrant := WithGitHubTools(db, parentDot.ID, base, service).(model.Generator)
	if !containsTool(parentWithGrant.Config.Tools, "aether_github_get_repository") {
		t.Fatal("authorized Dot did not receive the read-only GitHub catalog")
	}
	childWithParentProvider := WithGitHubTools(db, childDot.ID, base, service).(model.Generator)
	if containsTool(childWithParentProvider.Config.Tools, "aether_github_get_repository") {
		t.Fatal("a specialist without its own grant inherited the parent's repository access")
	}
	if err := db.UpdateConversationProfile(parentConversation.ID, spaceID, parentConversation.Name, parentConversation.Icon,
		parentConversation.Color, parentConversation.Purpose, parentConversation.Instructions, []string{childConversation.ID}); err != nil {
		t.Fatal(err)
	}
	delegatedProvider := WithSpecialistDelegationFactory(db, parentDot.ID, parentWithGrant, func(id string) model.Provider {
		return WithGitHubTools(db, id, base, service)
	}, nil).(model.Generator)
	if !containsTool(delegatedProvider.Config.Tools, "aether_github_get_repository") {
		t.Fatal("parent GitHub capabilities were not retained by the parent provider")
	}
	if !containsTool(delegatedProvider.Config.Tools, "aether_delegate_to_dot") {
		t.Fatal("permissioned specialist handoff was not registered")
	}

	tool := findTool(t, parentWithGrant.Config.Tools, "aether_github_get_repository")
	if _, err := tool.Handler(context.Background(), `{"repository_id":202}`); err == nil {
		t.Fatal("model-supplied repository B bypassed the repository A enum and grant")
	}
	var requests func() int
	service, requests = testGitHubToolService(t)
	parentWithGrant = WithGitHubTools(db, parentDot.ID, base, service).(model.Generator)
	tool = findTool(t, parentWithGrant.Config.Tools, "aether_github_get_repository")
	result, err := tool.Handler(context.Background(), `{"repository_id":101}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"untrusted_external_data":true`) || !strings.Contains(result, "https://github.com/octocat/alpha") || !strings.Contains(result, "latest-commit") {
		t.Fatalf("GitHub tool result lacked source links or the external-data marker: %s", result)
	}
	if requests() != 3 {
		t.Fatalf("repository inspection made %d requests, want one access check plus metadata and commit", requests())
	}

	if err := db.SetConversationGitHubGrants(parentConversation.ID, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Handler(context.Background(), `{"repository_id":101}`); err == nil {
		t.Fatal("revoked repository grant remained usable through a stale tool catalog")
	}
	if requests() != 3 {
		t.Fatalf("revoked request reached GitHub: request count=%d", requests())
	}
	if err := db.DisconnectGitHub(); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Handler(context.Background(), `{"repository_id":101}`); err == nil {
		t.Fatal("disconnect left a stale GitHub tool usable")
	}
	if requests() != 3 {
		t.Fatalf("disconnected request reached GitHub: request count=%d", requests())
	}
	events, ok := db.GetDot(parentDot.ID)
	if !ok || len(events.Events) == 0 || events.Events[len(events.Events)-1].Type != "github_read" {
		t.Fatalf("GitHub read activity was not recorded in the Dot event chain: %#v", events.Events)
	}
}

func TestGitHubToolArgumentsAreStrictAndBounded(t *testing.T) {
	for _, arguments := range []string{
		`{"repository_id":101,"owner":"attacker"}`,
		`{"repository_id":101} {"repository_id":101}`,
		strings.Repeat(" ", 12*1024+1),
	} {
		var target struct {
			RepositoryID int64 `json:"repository_id"`
		}
		if err := decodeGitHubArguments(arguments, &target); err == nil {
			t.Fatalf("accepted invalid or oversized arguments: %q", arguments[:min(40, len(arguments))])
		}
	}
	encoded, err := encodeGitHubToolResult(map[string]any{"summary": strings.Repeat("x", maxGitHubToolResultBytes)})
	if err == nil || encoded != "" {
		t.Fatalf("oversized GitHub result was accepted: bytes=%d err=%v", len(encoded), err)
	}
}

func TestGitHubRepositoryListingChecksCurrentAccessAndEnforcesLimits(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "Repository Dot"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Review selected repositories")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Running, "test run started"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Done, "test run completed"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	repositories := []task.GitHubRepository{githubToolRepo(101, "alpha"), githubToolRepo(202, "beta")}
	if err := db.ReplaceGitHubRepositories(repositories); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101, 202}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101, 202}, true); err != nil {
		t.Fatal(err)
	}
	service, requests := testGitHubToolService(t)
	provider := WithGitHubTools(db, dot.ID, model.Generator{Config: model.Config{Model: "test-model"}}, service).(model.Generator)
	list := findTool(t, provider.Config.Tools, "aether_github_list_repositories")
	result, err := list.Handler(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "octocat/alpha") || strings.Contains(result, "octocat/beta") {
		t.Fatalf("repository listing did not match the GitHub CLI account's current access: %s", result)
	}
	if requests() != 2 {
		t.Fatalf("listing two granted repositories made %d access checks, want one each", requests())
	}

	issues := findTool(t, provider.Config.Tools, "aether_github_list_issues")
	if _, err := issues.Handler(context.Background(), `{"repository_id":101,"limit":6}`); err == nil {
		t.Fatal("issue tool accepted a result limit larger than its declared five-item maximum")
	}
	if requests() != 2 {
		t.Fatalf("invalid issue limit reached GitHub; request count=%d", requests())
	}
}

func containsTool(tools []model.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func findTool(t *testing.T, tools []model.Tool, name string) model.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %#v", name, tools)
	return model.Tool{}
}

// testGitHubToolService installs a fake gh that serves a small repository and
// returns a Service plus a counter of API calls (the /user session check is not
// counted).
func testGitHubToolService(t *testing.T) (*github.Service, func() int) {
	t.Helper()
	log := ghtest.Install(t, true,
		ghtest.Route{Glob: "/repositories/101", Out: `{"id":101}`},
		ghtest.Route{Glob: "/repos/octocat/alpha", Out: `{"id":101,"name":"alpha","full_name":"octocat/alpha","description":"A test repo","html_url":"https://github.com/octocat/alpha","private":true,"default_branch":"main"}`},
		ghtest.Route{Glob: "/repos/octocat/alpha/commits/main", Out: `{"sha":"latest-commit","html_url":"https://github.com/octocat/alpha/commit/latest-commit","commit":{"message":"Add a feature","author":{"name":"Octo Cat","date":"2026-11-02T12:00:00Z"}}}`},
	)
	return github.NewService(), func() int { return len(ghtest.APICalls(log)) }
}

func githubToolRepo(id int64, name string) task.GitHubRepository {
	return task.GitHubRepository{
		ID: id, Owner: "octocat", Name: name,
		FullName: "octocat/" + name, HTMLURL: "https://github.com/octocat/" + name,
		Private: true, DefaultBranch: "main",
	}
}

func TestGitHubToolOutputIsValidJSON(t *testing.T) {
	value := map[string]any{"untrusted_external_data": true, "items": []string{"one", "two"}}
	encoded, err := encodeGitHubToolResult(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["untrusted_external_data"] != true {
		t.Fatalf("invalid result marker: %#v", decoded)
	}
}

func TestGitHubToolsStopWhenGHAccountChanges(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "Repository Dot"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceGitHubRepositories([]task.GitHubRepository{githubToolRepo(101, "alpha")}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitHubRepositorySelection([]int64{101}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConversationGitHubGrants(conversation.ID, []int64{101}, true); err != nil {
		t.Fatal(err)
	}
	service, _ := testGitHubToolService(t) // gh is signed in as "octocat"
	base := model.Generator{Config: model.Config{Model: "test-model"}}
	if err := db.SetGitHubAccount("octocat"); err != nil {
		t.Fatal(err)
	}
	if !containsTool(WithGitHubTools(db, dot.ID, base, service).(model.Generator).Config.Tools, "aether_github_get_repository") {
		t.Fatal("tools missing for the account AETHER was connected with")
	}
	// gh is shared machine state: after `gh auth switch` it reports another
	// login, and the previous account's grants must stop applying.
	ghtest.InstallAs(t, "mallory", ghtest.Route{Glob: "/repositories/101", Out: `{"id":101}`})
	switched := github.NewService()
	if containsTool(WithGitHubTools(db, dot.ID, base, switched).(model.Generator).Config.Tools, "aether_github_get_repository") {
		t.Fatal("GitHub tools stayed active although the gh account differs from the connected one")
	}
}
