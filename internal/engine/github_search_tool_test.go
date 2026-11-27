package engine

import (
	"context"
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func TestSearchIssuesToolIsGrantedPerRepositoryAndPinned(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conv, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Issue triage"})
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
	dot, _, err := db.CreateConversationRun(conv.ID, "Triage")
	if err != nil {
		t.Fatal(err)
	}
	log := ghtest.Install(t, true,
		ghtest.Route{Glob: "/repositories/101", Out: `{"id":101}`},
		ghtest.Route{Glob: `/search/issues*`, Out: `{"items":[{"number":4,"title":"Crash on start","state":"closed","html_url":"https://github.com/octocat/alpha/issues/4"}]}`})
	tools := WithGitHubTools(db, dot.ID, model.Generator{Config: model.Config{Model: "test-model"}}, github.NewService()).(model.Generator)
	search := findTool(t, tools.Config.Tools, "aether_github_search_issues")

	if _, err := search.Handler(context.Background(), `{"repository_id":202,"query":"crash","limit":3}`); err == nil {
		t.Fatal("searched a repository the agent cannot read")
	}
	if _, err := search.Handler(context.Background(), `{"repository_id":101,"query":"repo:evil/secret","limit":3}`); err == nil {
		t.Fatal("searched with only a qualifier")
	}
	if _, err := search.Handler(context.Background(), `{"repository_id":101,"query":"crash","limit":3,"extra":1}`); err == nil {
		t.Fatal("accepted an unknown argument")
	}
	out, err := search.Handler(context.Background(), `{"repository_id":101,"query":"crash repo:evil/secret","limit":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"untrusted_external_data":true`) || !strings.Contains(out, "Crash on start") || strings.Contains(out, "evil") {
		t.Fatalf("result = %s", out)
	}
	var searches []string
	for _, call := range ghtest.APICalls(log) {
		if strings.HasPrefix(call, "/search/issues") {
			searches = append(searches, call)
		}
	}
	if len(searches) != 1 || !strings.Contains(searches[0], "q=repo%3Aoctocat%2Falpha+is%3Aissue+crash") || strings.Contains(searches[0], "evil") {
		t.Fatalf("search calls = %q", searches)
	}
	current, _ := db.GetDot(dot.ID)
	recorded := false
	for _, event := range current.Events {
		if event.Type == "github_read" && strings.Contains(event.Message, "Searched GitHub issues in octocat/alpha") {
			recorded = true
		}
	}
	if !recorded {
		t.Fatal("search was not recorded in the agent's event chain")
	}
}
