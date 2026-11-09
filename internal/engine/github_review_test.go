package engine

import (
	"strings"
	"testing"

	"aether-dots/internal/task"
)

func TestProposalWarnings(t *testing.T) {
	cases := []struct {
		name   string
		p      task.GitHubProposal
		public bool
		want   []string
		absent []string
	}{
		{"plain text", task.GitHubProposal{Kind: task.GitHubIssueComment, Body: "Thanks, merged."}, false, nil, []string{"Mentions", "closing", "links", "public", "outside"}},
		{"mentions but not emails or code", task.GitHubProposal{Kind: task.GitHubIssueComment, Body: "cc @alice and @org/team, mail bob@example.com, `@code`"}, false, []string{"Mentions @alice, @org/team"}, []string{"bob"}},
		{"closing keyword only matters on a PR", task.GitHubProposal{Kind: task.GitHubPRDraft, Body: "Fixes #12"}, false, []string{"closing keyword"}, nil},
		{"closing keyword in a comment is harmless", task.GitHubProposal{Kind: task.GitHubIssueComment, Body: "this fixes #12"}, false, nil, []string{"closing"}},
		{"links and images", task.GitHubProposal{Kind: task.GitHubIssueCreate, Body: "see https://example.com ![x](y)"}, false, []string{"links or images"}, nil},
		{"untrusted run and public repo", task.GitHubProposal{Kind: task.GitHubIssueCreate, Body: "x", ReadUntrustedInRun: true}, true, []string{"outside content", "public repository"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			joined := strings.Join(ProposalWarnings(c.p, c.public), "\n")
			for _, w := range c.want {
				if !strings.Contains(joined, w) {
					t.Fatalf("missing %q in %q", w, joined)
				}
			}
			for _, w := range c.absent {
				if strings.Contains(joined, w) {
					t.Fatalf("unexpected %q in %q", w, joined)
				}
			}
		})
	}
}
