package engine

import (
	"fmt"
	"regexp"
	"strings"

	"aether-dots/internal/task"
)

var (
	proposalMentionPattern = regexp.MustCompile("(?:^|[^A-Za-z0-9_`/])@([A-Za-z0-9][A-Za-z0-9-]{0,38})(?:/([A-Za-z0-9_-]+))?")
	proposalClosingPattern = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+(?:[\w.-]+/[\w.-]+)?#\d+`)
	proposalLinkPattern    = regexp.MustCompile(`(?i)https?://|!\[`)
)

// ProposalWarnings lists what a reviewer should double-check. It is derived
// from the content and provenance only, and never blocks approval.
func ProposalWarnings(p task.GitHubProposal, public bool) []string {
	var out []string
	if p.ReadUntrustedInRun {
		out = append(out, "The Dot read GitHub or other outside content during this run. That text may have influenced this proposal; read it as if a stranger wrote it.")
	}
	if public {
		out = append(out, "This is a public repository: anyone can read what you publish.")
	}
	text := p.Title + "\n" + p.Body
	seen := map[string]bool{}
	var mentions []string
	for _, m := range proposalMentionPattern.FindAllStringSubmatch(text, -1) {
		name := "@" + m[1]
		if m[2] != "" {
			name += "/" + m[2]
		}
		if !seen[strings.ToLower(name)] && len(mentions) < 5 {
			seen[strings.ToLower(name)] = true
			mentions = append(mentions, name)
		}
	}
	if len(mentions) > 0 {
		out = append(out, "Mentions "+strings.Join(mentions, ", ")+": GitHub will notify them.")
	}
	if p.Kind == task.GitHubPRDraft && proposalClosingPattern.MatchString(text) {
		out = append(out, "Contains a closing keyword (for example \"fixes #12\"): merging this pull request will close that issue.")
	}
	if proposalLinkPattern.MatchString(text) {
		out = append(out, "Contains links or images. They are shown here as plain text and not loaded.")
	}
	return out
}

// ProposalHeadline is the one-line description of what a proposal does.
func ProposalHeadline(p task.GitHubProposal) string {
	switch p.Kind {
	case task.GitHubIssueComment:
		return fmt.Sprintf("Comment on #%d", p.Target)
	case task.GitHubIssueCreate:
		return "New issue"
	case task.GitHubPRDraft:
		return "Draft pull request"
	}
	return string(p.Kind)
}
