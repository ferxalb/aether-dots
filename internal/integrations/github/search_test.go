package github

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"aether-dots/internal/integrations/github/ghtest"
)

func TestSearchTermsDropQualifiersAndOperators(t *testing.T) {
	got := SearchTerms(`crash repo:evil/x OR org:other "null pointer" -label:bug is:pr NOT panic() v1.2`)
	want := []string{"crash", "null", "pointer", "panic", "v1.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchTerms = %q, want %q", got, want)
	}
	if got := SearchTerms("repo:a/b org:c user:d"); len(got) != 0 {
		t.Fatalf("qualifier-only query kept %q", got)
	}
	if got := SearchTerms(strings.Repeat("word ", 20)); len(got) != maxSearchTerms {
		t.Fatalf("kept %d terms, want %d", len(got), maxSearchTerms)
	}
	if got := SearchTerms(strings.Repeat("x", 100)); len(got[0]) != maxSearchTermBytes {
		t.Fatalf("term length %d", len(got[0]))
	}
}

func TestSearchIssuesIsPinnedToOneRepositoryAndSkipsPullRequests(t *testing.T) {
	s, log := newFakeGH(t, true, route{Glob: `/search/issues*`, Out: `{"total_count":3,"items":[` +
		`{"number":4,"title":"Crash on start","state":"closed","html_url":"https://github.com/octocat/alpha/issues/4","body":"` + strings.Repeat("b", 900) + `"},` +
		`{"number":5,"title":"Fix crash","state":"open","html_url":"https://github.com/octocat/alpha/pull/5","pull_request":{"html_url":"https://github.com/octocat/alpha/pull/5"}},` +
		`{"number":6,"title":"Crash again","state":"open","html_url":"https://github.com/octocat/alpha/issues/6"}]}`})
	issues, err := s.SearchIssues(context.Background(), "octocat", "alpha", "crash repo:evil/secret org:evil", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Number != 4 || issues[1].Number != 6 || len(issues[0].Body) > 300 {
		t.Fatalf("issues = %+v", issues)
	}
	calls := ghtest.APICalls(log)
	if len(calls) != 1 {
		t.Fatalf("calls = %q", calls)
	}
	if want := "/search/issues?page=1&per_page=5&q=repo%3Aoctocat%2Falpha+is%3Aissue+crash"; calls[0] != want {
		t.Fatalf("endpoint = %q, want %q", calls[0], want)
	}
	if _, err := s.SearchIssues(context.Background(), "octocat", "alpha", "repo:evil/secret", 5); err == nil {
		t.Fatal("searched with only a qualifier")
	}
	if _, err := s.SearchIssues(context.Background(), "octocat", "alpha", "crash", 6); err == nil {
		t.Fatal("accepted a limit above 5")
	}
	if _, err := s.SearchIssues(context.Background(), "octo cat", "alpha", "crash", 5); err == nil {
		t.Fatal("accepted an invalid owner")
	}
	if got := len(ghtest.APICalls(log)); got != 1 {
		t.Fatalf("refused searches still called gh: %d calls", got)
	}
}
