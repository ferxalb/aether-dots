package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

type githubToolCredentialStore struct {
	mu          sync.Mutex
	credentials github.Credentials
	deleted     bool
}

func (s *githubToolCredentialStore) Load(ctx context.Context) (github.Credentials, error) {
	if err := ctx.Err(); err != nil {
		return github.Credentials{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleted || s.credentials.AccessToken == "" {
		return github.Credentials{}, github.ErrNotConnected
	}
	return s.credentials, nil
}

func (s *githubToolCredentialStore) Save(ctx context.Context, credentials github.Credentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.credentials = credentials
	s.deleted = false
	s.mu.Unlock()
	return nil
}

func (s *githubToolCredentialStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.deleted = true
	s.credentials = github.Credentials{}
	s.mu.Unlock()
	return nil
}

type githubToolRoundTripFunc func(*http.Request) (*http.Response, error)

func (f githubToolRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

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
	service := testGitHubToolService(nil, nil)

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
	var requests atomic.Int32
	service = testGitHubToolService(&requests, nil)
	parentWithGrant = WithGitHubTools(db, parentDot.ID, base, service).(model.Generator)
	tool = findTool(t, parentWithGrant.Config.Tools, "aether_github_get_repository")
	result, err := tool.Handler(context.Background(), `{"repository_id":101}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"untrusted_external_data":true`) || !strings.Contains(result, "https://github.com/octocat/alpha") || !strings.Contains(result, "latest-commit") {
		t.Fatalf("GitHub tool result lacked source links or the external-data marker: %s", result)
	}
	if requests.Load() != 3 {
		t.Fatalf("repository inspection made %d requests, want one access check plus metadata and commit", requests.Load())
	}

	if err := db.SetConversationGitHubGrants(parentConversation.ID, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Handler(context.Background(), `{"repository_id":101}`); err == nil {
		t.Fatal("revoked repository grant remained usable through a stale tool catalog")
	}
	if requests.Load() != 3 {
		t.Fatalf("revoked request reached GitHub: request count=%d", requests.Load())
	}
	if err := db.DisconnectGitHub(); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Handler(context.Background(), `{"repository_id":101}`); err == nil {
		t.Fatal("disconnect left a stale GitHub tool usable")
	}
	if requests.Load() != 3 {
		t.Fatalf("disconnected request reached GitHub: request count=%d", requests.Load())
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

func TestGitHubRepositoryListingBatchesInstallationChecksAndEnforcesLimits(t *testing.T) {
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
	var requests atomic.Int32
	provider := WithGitHubTools(db, dot.ID, model.Generator{Config: model.Config{Model: "test-model"}}, testGitHubToolService(&requests, nil)).(model.Generator)
	list := findTool(t, provider.Config.Tools, "aether_github_list_repositories")
	result, err := list.Handler(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "octocat/alpha") || strings.Contains(result, "octocat/beta") {
		t.Fatalf("repository listing did not match GitHub's current installation access: %s", result)
	}
	if requests.Load() != 1 {
		t.Fatalf("listing repositories made %d installation checks for one installation", requests.Load())
	}

	issues := findTool(t, provider.Config.Tools, "aether_github_list_issues")
	if _, err := issues.Handler(context.Background(), `{"repository_id":101,"limit":6}`); err == nil {
		t.Fatal("issue tool accepted a result limit larger than its declared five-item maximum")
	}
	if requests.Load() != 1 {
		t.Fatalf("invalid issue limit reached GitHub; request count=%d", requests.Load())
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

func testGitHubToolService(requests *atomic.Int32, ready chan struct{}) *github.Service {
	vault := &githubToolCredentialStore{credentials: github.Credentials{AccessToken: "ghu_test"}}
	httpClient := &http.Client{Transport: githubToolRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if requests != nil {
			requests.Add(1)
		}
		if request.Header.Get("Authorization") != "Bearer ghu_test" {
			return nil, errors.New("unexpected GitHub access token")
		}
		if ready != nil {
			select {
			case <-ready:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		}
		var body string
		status := http.StatusOK
		switch {
		case request.Method != http.MethodGet:
			return nil, errors.New("GitHub client attempted a non-read operation")
		case request.URL.Path == "/user/installations/9001/repositories":
			body = `{"total_count":1,"repositories":[{"id":101,"name":"alpha","full_name":"octocat/alpha","html_url":"https://github.com/octocat/alpha","private":true,"owner":{"login":"octocat"}}]}`
		case request.URL.Path == "/repos/octocat/alpha":
			body = `{"id":101,"name":"alpha","full_name":"octocat/alpha","description":"A test repo","html_url":"https://github.com/octocat/alpha","private":true,"default_branch":"main"}`
		case request.URL.Path == "/repos/octocat/alpha/commits/main":
			body = `{"sha":"latest-commit","html_url":"https://github.com/octocat/alpha/commit/latest-commit","commit":{"message":"Add a feature","author":{"name":"Octo Cat","date":"2026-11-02T12:00:00Z"}}}`
		default:
			status = http.StatusNotFound
			body = `{"message":"Not Found"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return github.NewService("public-client", vault, httpClient)
}

func githubToolRepo(id int64, name string) task.GitHubRepository {
	return task.GitHubRepository{
		ID: id, InstallationID: 9001, Owner: "octocat", Name: name,
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
