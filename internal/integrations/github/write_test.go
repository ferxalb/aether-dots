package github

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/integrations/github/ghtest"
)

const (
	acceptArgs = "-H Accept: application/vnd.github+json -H X-GitHub-Api-Version: " + APIVersion
	commentURL = "https://github.com/octocat/alpha/issues/7#issuecomment-99"
)

// repoReply is GET /repositories/101 as gh prints it with -i. scopes "-" omits
// the header (fine-grained tokens send none).
func repoReply(t *testing.T, scopes string, remaining string, patch map[string]any) string {
	t.Helper()
	repo := map[string]any{
		"id": 101, "name": "alpha", "full_name": "octocat/alpha", "private": true,
		"archived": false, "disabled": false, "has_issues": true,
		"owner": map[string]any{"login": "octocat"},
	}
	for k, v := range patch {
		repo[k] = v
	}
	head := "HTTP/2.0 200 OK\r\nContent-Type: application/json\r\n"
	if scopes != "-" {
		head += "X-Oauth-Scopes: " + scopes + "\r\n"
	}
	return head + "X-Ratelimit-Remaining: " + remaining + "\r\n\r\n" + jsonOf(t, repo)
}

func okRepo(t *testing.T) route {
	return route{Glob: `/repositories/101`, Out: repoReply(t, "gist, repo, read:org", "4990", nil)}
}

func commentRoutes(t *testing.T, reply string) []route {
	return []route{
		okRepo(t),
		{Glob: `/repos/octocat/alpha/issues/7`, Out: `{"number":7,"locked":false}`},
		{Glob: `/repos/octocat/alpha/issues/7/comments`, Out: reply},
	}
}

func commentReply(t *testing.T, patch map[string]any) string {
	r := map[string]any{"html_url": commentURL, "id": 99, "user": map[string]any{"login": "octocat"}}
	for k, v := range patch {
		r[k] = v
	}
	return jsonOf(t, r)
}

func posts(log string) []string {
	b, _ := os.ReadFile(log)
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.Contains(line, "--method POST") {
			out = append(out, line)
		}
	}
	return out
}

// assertOnlyFixedCalls enforces the whole boundary: every api call is either
// a pinned GET or the single allow-listed POST shape, on github.com.
func assertOnlyFixedCalls(t *testing.T, log string) {
	t.Helper()
	b, _ := os.ReadFile(log)
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if !strings.HasPrefix(line, "github.com|api ") {
			t.Fatalf("unexpected process or host: %s", line)
		}
		switch {
		case strings.Contains(line, "--method GET"):
		case strings.Contains(line, "--method POST"):
			if !strings.Contains(line, " --input - /repos/") || strings.Contains(line, " -f ") || strings.Contains(line, " -F ") {
				t.Fatalf("POST outside the fixed shape: %s", line)
			}
		default:
			t.Fatalf("api call with no pinned method: %s", line)
		}
	}
}

func TestWriteCommentSendsExactlyTheApprovedBodyOnStdin(t *testing.T) {
	s, log := newFakeGH(t, true, commentRoutes(t, commentReply(t, nil))...)
	body := "@/etc/passwd -f x=y --method DELETE \"quoted\"\nline two"
	res, err := s.Write(context.Background(), "OctoCat", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: body})
	if err != nil || res.URL != commentURL || res.Number != 7 {
		t.Fatalf("res=%#v err=%v", res, err)
	}
	p := posts(log)
	if len(p) != 1 || p[0] != "github.com|api --method POST "+acceptArgs+" --input - /repos/octocat/alpha/issues/7/comments" {
		t.Fatalf("POST argv: %q", p)
	}
	if strings.Contains(p[0], "passwd") {
		t.Fatal("the body reached argv")
	}
	if got, want := ghtest.Stdin(log), `{"body":"@/etc/passwd -f x=y --method DELETE \"quoted\"\nline two"}`; got != want {
		t.Fatalf("stdin %q, want %q", got, want)
	}
	assertOnlyFixedCalls(t, log)
}

func TestWriteIssueAndDraftPullRequestUseFixedEndpointsAndShapes(t *testing.T) {
	s, log := newFakeGH(t, true,
		okRepo(t),
		route{Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/issues/12", "number": 12, "user": map[string]any{"login": "octocat"}})},
		route{Glob: `/repos/octocat/alpha/git/ref/heads/feat/x`, Out: `{"ref":"refs/heads/feat/x"}`},
		route{Glob: `/repos/octocat/alpha/git/ref/heads/main`, Out: `{"ref":"refs/heads/main"}`},
		route{Glob: `/repos/octocat/alpha/pulls\?*`, Out: `[]`},
		route{Glob: `/repos/octocat/alpha/pulls`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/pull/13", "number": 13, "draft": true, "user": map[string]any{"login": "octocat"}})},
	)
	issue, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "Bug", Body: "It breaks"})
	if err != nil || issue.Number != 12 {
		t.Fatalf("issue %#v %v", issue, err)
	}
	pr, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WritePRDraft, RepositoryID: 101, Title: "T", Body: "B", Head: "feat/x", Base: "main"})
	if err != nil || pr.Number != 13 || !strings.HasSuffix(pr.URL, "/pull/13") {
		t.Fatalf("pr %#v %v", pr, err)
	}
	if got, want := ghtest.Stdin(log), `{"title":"Bug","body":"It breaks"}`+`{"title":"T","body":"B","head":"feat/x","base":"main","draft":true}`; got != want {
		t.Fatalf("stdin %q", got)
	}
	assertOnlyFixedCalls(t, log)
}

func TestWriteUsesTheCurrentRepositoryNameForARenamedRepository(t *testing.T) {
	s, log := newFakeGH(t, true,
		route{Glob: `/repositories/101`, Out: repoReply(t, "repo", "4000", map[string]any{"full_name": "octocat/renamed", "name": "renamed"})},
		route{Glob: `/repos/octocat/renamed/issues/7`, Out: `{"number":7}`},
		route{Glob: `/repos/octocat/renamed/issues/7/comments`, Out: commentReply(t, map[string]any{"html_url": "https://github.com/octocat/renamed/issues/7#issuecomment-5"})},
	)
	if _, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if p := posts(log); len(p) != 1 || !strings.HasSuffix(p[0], "/repos/octocat/renamed/issues/7/comments") {
		t.Fatalf("%q", p)
	}
}

func TestWriteRefusesBeforeSendingWhenAReadCheckFails(t *testing.T) {
	prReq := WriteRequest{Kind: WritePRDraft, RepositoryID: 101, Title: "T", Head: "feat/x", Base: "main"}
	cases := []struct {
		name   string
		login  string
		req    WriteRequest
		routes []route
		want   error
	}{
		{"other account", "mallory", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"}, commentRoutes(t, commentReply(t, nil)), ErrAccountChanged},
		{"token without repo scope", "octocat", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "gist, read:org", "4000", nil)}}, ErrMissingScope},
		{"public_repo is not enough for a private repo", "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "public_repo", "4000", nil)}}, ErrMissingScope},
		{"rate limit nearly exhausted", "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "repo", "1", nil)}}, ErrRateLimited},
		{"repository id mismatch", "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "repo", "4000", map[string]any{"id": 999})}}, ErrPermissionDenied},
		{"issues disabled", "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "repo", "4000", map[string]any{"has_issues": false})}}, nil},
		{"archived", "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"},
			[]route{{Glob: `/repositories/101`, Out: repoReply(t, "repo", "4000", map[string]any{"archived": true})}}, nil},
		{"locked conversation", "octocat", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"},
			[]route{okRepo(t), {Glob: `/repos/octocat/alpha/issues/7`, Out: `{"number":7,"locked":true}`}}, nil},
		{"missing issue", "octocat", WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"},
			[]route{okRepo(t)}, ErrNotFound},
		{"missing head branch", "octocat", prReq,
			[]route{okRepo(t), {Glob: `/repos/octocat/alpha/git/ref/heads/main`, Out: `{"ref":"x"}`}}, nil},
		{"open pull request already exists", "octocat", prReq,
			[]route{okRepo(t),
				{Glob: `/repos/octocat/alpha/git/ref/heads/feat/x`, Out: `{"ref":"x"}`}, {Glob: `/repos/octocat/alpha/git/ref/heads/main`, Out: `{"ref":"x"}`},
				{Glob: `/repos/octocat/alpha/pulls\?*`, Out: `[{"html_url":"https://github.com/octocat/alpha/pull/3"}]`}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			binary, log := ghtest.ScriptAs(t, c.login, c.routes...)
			s := &Service{gh: &ghRunner{path: binary}, operations: map[uint64]context.CancelFunc{}}
			_, err := s.Write(context.Background(), "octocat", c.req)
			if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
				t.Fatalf("err=%v want %v", err, c.want)
			}
			if IsUncertainWrite(err) {
				t.Fatal("a refusal before sending was reported as uncertain")
			}
			if n := len(posts(log)); n != 0 {
				t.Fatalf("%d POSTs went out after a failed check", n)
			}
			assertOnlyFixedCalls(t, log)
		})
	}
}

func TestPublicRepoScopeSufficesForPublicRepository(t *testing.T) {
	s, _ := newFakeGH(t, true,
		route{Glob: `/repositories/101`, Out: repoReply(t, "public_repo", "4000", map[string]any{"private": false})},
		route{Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/issues/1", "number": 1, "user": map[string]any{"login": "octocat"}})},
	)
	if _, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestFineGrainedTokenWithoutScopeHeaderIsNotBlockedByTheScopeCheck(t *testing.T) {
	s, _ := newFakeGH(t, true,
		route{Glob: `/repositories/101`, Out: repoReply(t, "-", "4000", nil)},
		route{Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/issues/1", "number": 1, "user": map[string]any{"login": "octocat"}})},
	)
	if _, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteClassifiesFailuresAfterTheRequestStarted(t *testing.T) {
	post := func(code int, stderr string) []route {
		return []route{okRepo(t), {Glob: `/repos/octocat/alpha/issues`, Code: code, Stderr: stderr}}
	}
	req := WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"}
	cases := []struct {
		name      string
		routes    []route
		uncertain bool
		want      string
	}{
		{"validation failed", post(1, "gh: Validation Failed (HTTP 422)"), false, "rejected the content"},
		{"forbidden", post(1, "gh: Forbidden (HTTP 403)"), false, "refused"},
		{"not found", post(1, "gh: Not Found (HTTP 404)"), false, "could not find"},
		{"gone", post(1, "gh: Gone (HTTP 410)"), false, "disabled"},
		{"rate limited", post(1, "gh: API rate limit exceeded (HTTP 403)"), false, "rate limit"},
		{"server error", post(1, "gh: Bad Gateway (HTTP 502)"), true, "may have been applied"},
		{"no status at all", post(1, "dial tcp: i/o timeout"), true, "did not confirm"},
		{"garbage confirmation", []route{okRepo(t), {Glob: `/repos/octocat/alpha/issues`, Out: `not json`}}, true, "unreadable"},
		{"link into another repository", []route{okRepo(t), {Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/evil/beta/issues/1", "number": 1, "user": map[string]any{"login": "octocat"}})}}, true, "outside"},
		{"link to another host", []route{okRepo(t), {Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://evil.example/octocat/alpha/issues/1", "number": 1, "user": map[string]any{"login": "octocat"}})}}, true, "unexpected link"},
		{"author is someone else", []route{okRepo(t), {Glob: `/repos/octocat/alpha/issues`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/issues/1", "number": 1, "user": map[string]any{"login": "mallory"}})}}, true, "different author"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, log := newFakeGH(t, true, c.routes...)
			_, err := s.Write(context.Background(), "octocat", req)
			if err == nil || !strings.Contains(err.Error(), c.want) || IsUncertainWrite(err) != c.uncertain {
				t.Fatalf("err=%v uncertain=%v, want %q uncertain=%v", err, IsUncertainWrite(err), c.want, c.uncertain)
			}
			if n := len(posts(log)); n != 1 {
				t.Fatalf("expected exactly one POST and no retry, got %d", n)
			}
		})
	}
}

func TestDraftPullRequestThatIsNotADraftIsReportedUncertain(t *testing.T) {
	s, _ := newFakeGH(t, true,
		okRepo(t),
		route{Glob: `/repos/octocat/alpha/git/ref/heads/feat`, Out: `{"ref":"x"}`}, route{Glob: `/repos/octocat/alpha/git/ref/heads/main`, Out: `{"ref":"x"}`},
		route{Glob: `/repos/octocat/alpha/pulls\?*`, Out: `[]`},
		route{Glob: `/repos/octocat/alpha/pulls`, Out: jsonOf(t, map[string]any{"html_url": "https://github.com/octocat/alpha/pull/4", "number": 4, "draft": false, "user": map[string]any{"login": "octocat"}})},
	)
	_, err := s.Write(context.Background(), "octocat", WriteRequest{Kind: WritePRDraft, RepositoryID: 101, Title: "T", Head: "feat", Base: "main"})
	if !IsUncertainWrite(err) || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("err=%v", err)
	}
}

func TestCancelledWriteIsUncertainAndNeverRetried(t *testing.T) {
	s, log := newFakeGH(t, true, okRepo(t), route{Glob: `/repos/octocat/alpha/issues`, Hang: true})
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.Write(ctx, "octocat", WriteRequest{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t"})
	if !IsUncertainWrite(err) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("the hung gh process was not killed")
	}
	if n := len(posts(log)); n != 1 {
		t.Fatalf("%d POSTs", n)
	}
}

func TestInvalidWriteRequestsNeverReachGH(t *testing.T) {
	bad := []WriteRequest{
		{Kind: "delete_repo", RepositoryID: 101, Title: "t"},
		{Kind: WriteIssueComment, RepositoryID: 101, Body: "no target"},
		{Kind: WriteIssueComment, RepositoryID: 101, Target: 1, Body: " "},
		{Kind: WriteIssueCreate, RepositoryID: 101, Title: "a\nb"},
		{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t", Head: "x"},
		{Kind: WriteIssueCreate, Title: "t"},
		{Kind: WriteIssueCreate, RepositoryID: 101, Title: "t", Body: strings.Repeat("x", maxWriteBodyBytes+1)},
		{Kind: WritePRDraft, RepositoryID: 101, Title: "t", Head: "--flag", Base: "main"},
		{Kind: WritePRDraft, RepositoryID: 101, Title: "t", Head: "a/../b", Base: "main"},
		{Kind: WritePRDraft, RepositoryID: 101, Title: "t", Head: "main", Base: "main"},
	}
	s, log := newFakeGH(t, true, okRepo(t))
	for i, req := range bad {
		if _, err := s.Write(context.Background(), "octocat", req); !errors.Is(err, ErrWriteInvalid) {
			t.Fatalf("request %d: err=%v", i, err)
		}
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "|api ") && strings.Contains(string(b), "/repositories/") {
		t.Fatalf("an invalid request caused API traffic:\n%s", b)
	}
}

func TestWriteIsBlockedWhenDisconnectedOrCLIMissing(t *testing.T) {
	s, log := newFakeGH(t, true, commentRoutes(t, commentReply(t, nil))...)
	s.CancelActiveRequests()
	req := WriteRequest{Kind: WriteIssueComment, RepositoryID: 101, Target: 7, Body: "x"}
	if _, err := s.Write(context.Background(), "octocat", req); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err=%v", err)
	}
	if n := len(posts(log)); n != 0 {
		t.Fatal("a disconnected service sent a write")
	}
	if _, err := (&Service{}).Write(context.Background(), "octocat", req); !errors.Is(err, ErrCLINotInstalled) {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildWriteRejectsPathsOutsideTheAllowList(t *testing.T) {
	for _, full := range []string{"octocat", "octo cat/alpha", "octocat/al/pha", "../x/y", "octocat/alpha;rm"} {
		if _, _, err := buildWrite(full, WriteRequest{Kind: WriteIssueCreate, Title: "t"}); err == nil {
			t.Fatalf("%q was accepted", full)
		}
	}
}
