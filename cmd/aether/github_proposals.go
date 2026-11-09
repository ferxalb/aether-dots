package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

// CLI parity for the GitHub approvals tray. Listing, reading, rejecting and
// resolving work anywhere; approving needs a person at an interactive
// terminal who types back part of the content hash they were just shown, and
// goes through the same engine.ApproveGitHubProposal as the desktop tray.

const minProposalIDPrefix = 6

var writeKindAliases = map[string]task.GitHubWriteKind{
	"comment": task.GitHubIssueComment, "issue_comment": task.GitHubIssueComment,
	"issue": task.GitHubIssueCreate, "issue_create": task.GitHubIssueCreate,
	"pr": task.GitHubPRDraft, "pr_draft": task.GitHubPRDraft, "draft": task.GitHubPRDraft,
}

// parseWriteGrantSpecs reads "<repo-id>:comment,issue,pr" arguments.
func parseWriteGrantSpecs(args []string) ([]task.GitHubWriteGrant, error) {
	var out []task.GitHubWriteGrant
	for _, arg := range args {
		idText, kindText, ok := strings.Cut(arg, ":")
		if !ok || strings.TrimSpace(kindText) == "" {
			return nil, fmt.Errorf("invalid write grant %q; use <repo-id>:comment,issue,pr", arg)
		}
		ids, err := parseGitHubRepositoryIDs([]string{idText})
		if err != nil {
			return nil, err
		}
		grant := task.GitHubWriteGrant{RepositoryID: ids[0]}
		for _, name := range strings.Split(kindText, ",") {
			kind, ok := writeKindAliases[strings.ToLower(strings.TrimSpace(name))]
			if !ok {
				return nil, fmt.Errorf("unknown GitHub write operation %q; use comment, issue, or pr", name)
			}
			grant.Kinds = append(grant.Kinds, kind)
		}
		out = append(out, grant)
	}
	return out, nil
}

func grantGitHubWrites(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("github write-grant", flag.ContinueOnError)
	allow := fs.Bool("allow-publish", false, "acknowledge that approved proposals are published under your GitHub account and may be public")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return errors.New("usage: aether github write-grant --allow-publish <dot-id> <repo-id>:comment,issue,pr...")
	}
	if !*allow {
		return errors.New("add --allow-publish after reviewing that approved proposals are published under your GitHub account")
	}
	grants, err := parseWriteGrantSpecs(rest[1:])
	if err != nil {
		return err
	}
	if err := db.SetConversationGitHubWriteGrants(rest[0], grants, true); err != nil {
		return err
	}
	fmt.Println("Saved. This Dot can now only propose; nothing is sent until you approve each proposal (`aether github proposals`).")
	return nil
}

func githubProposalsCommand(db *store.Store, service *github.Service, in io.Reader, out io.Writer, interactive bool, args []string) error {
	if err := db.SweepGitHubProposals(); err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		return listGitHubProposals(db, out)
	}
	if len(args) < 2 {
		return errors.New("usage: aether github proposals [list|show|approve|reject|resolve] <proposal-id>")
	}
	proposal, err := findProposalByPrefix(db, args[1])
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		printProposal(out, db, proposal)
		return nil
	case "reject":
		if err := db.RejectGitHubProposal(proposal.ID); err != nil {
			return err
		}
		fmt.Fprintln(out, "Rejected. Nothing was sent to GitHub.")
		return nil
	case "approve":
		return approveProposalInteractively(db, service, in, out, interactive, proposal)
	case "resolve":
		return resolveUncertainProposal(db, out, proposal, args[2:])
	default:
		return fmt.Errorf("unknown proposals command %q", args[0])
	}
}

func findProposalByPrefix(db *store.Store, prefix string) (task.GitHubProposal, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < minProposalIDPrefix {
		return task.GitHubProposal{}, fmt.Errorf("give at least %d characters of the proposal id", minProposalIDPrefix)
	}
	var found []task.GitHubProposal
	for _, p := range db.GitHubProposals() {
		if strings.HasPrefix(p.ID, prefix) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return task.GitHubProposal{}, store.ErrGitHubProposalNotFound
	case 1:
		return found[0], nil
	}
	return task.GitHubProposal{}, errors.New("that proposal id prefix is ambiguous; give more characters")
}

func listGitHubProposals(db *store.Store, out io.Writer) error {
	proposals := db.GitHubProposals()
	if len(proposals) == 0 {
		fmt.Fprintln(out, "No GitHub proposals.")
		return nil
	}
	names := map[string]string{}
	for _, c := range db.Snapshot().Conversations {
		names[c.ID] = c.Name
	}
	now := time.Now()
	for i := len(proposals) - 1; i >= 0; i-- {
		p := proposals[i]
		state := string(p.State)
		if p.State == task.GitHubProposalPending && !now.Before(p.ExpiresAt) {
			state = string(task.GitHubProposalExpired)
		}
		fmt.Fprintf(out, "%.12s\t%-9s\t%s\t%s\t%s\n", p.ID, state, terminalSafe(engine.ProposalHeadline(p)), terminalSafe(p.RepositoryName), terminalSafe(names[p.ConversationID]))
	}
	return nil
}

func printProposal(out io.Writer, db *store.Store, p task.GitHubProposal) {
	snapshot := db.Snapshot()
	name, public := "", false
	for _, c := range snapshot.Conversations {
		if c.ID == p.ConversationID {
			name = c.Name
		}
	}
	visibility := "private"
	for _, r := range snapshot.GitHubRepositories {
		if r.ID == p.RepositoryID && !r.Private {
			public, visibility = true, "public"
		}
	}
	state := string(p.State)
	if p.State == task.GitHubProposalPending {
		if left := time.Until(p.ExpiresAt); left > 0 {
			state += fmt.Sprintf(" (expires in %s)", left.Round(time.Minute))
		} else {
			state = string(task.GitHubProposalExpired)
		}
	}
	fmt.Fprintf(out, "Proposal   %s\nState      %s\nDot        %s\nOperation  %s\nRepository %s (%s)\nPublished as @%s\nContent    %s\n",
		p.ID, state, terminalSafe(name), terminalSafe(engine.ProposalHeadline(p)), terminalSafe(p.RepositoryName), visibility, terminalSafe(p.Account), p.ContentHash)
	if p.Kind == task.GitHubPRDraft {
		fmt.Fprintf(out, "Branches   %s -> %s (opens as a draft)\n", terminalSafe(p.Head), terminalSafe(p.Base))
	}
	if p.Title != "" {
		fmt.Fprintf(out, "Title      %s\n", terminalSafe(p.Title))
	}
	fmt.Fprintf(out, "--- text (exactly what will be published) ---\n%s\n--- end ---\n", terminalSafe(p.Body))
	if p.State == task.GitHubProposalPending {
		for _, w := range engine.ProposalWarnings(p, public) {
			fmt.Fprintf(out, "! %s\n", w)
		}
	}
	if p.Result != nil {
		fmt.Fprintf(out, "Published  %s\n", terminalSafe(p.Result.URL))
	}
	if p.FailureReason != "" {
		fmt.Fprintf(out, "Note       %s\n", terminalSafe(p.FailureReason))
	}
}

func approveProposalInteractively(db *store.Store, service *github.Service, in io.Reader, out io.Writer, interactive bool, shown task.GitHubProposal) error {
	if !interactive {
		return errors.New("approving publishes to GitHub and needs an interactive terminal; run this command yourself in a terminal")
	}
	if shown.State != task.GitHubProposalPending || !time.Now().Before(shown.ExpiresAt) {
		return errors.New("only a pending, unexpired proposal can be approved")
	}
	printProposal(out, db, shown)
	want := strings.ToLower(shown.ContentHash[:8])
	fmt.Fprintf(out, "\nTo publish this exactly as shown, type its content hash prefix (%s), or press Enter to cancel: ", want)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("cancelled: no confirmation was entered")
	}
	if strings.ToLower(strings.TrimSpace(line)) != want {
		return errors.New("cancelled: the confirmation did not match; nothing was sent")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	final, err := engine.ApproveGitHubProposal(ctx, db, service, shown.ID, shown.ContentHash)
	if err != nil {
		switch final.State {
		case task.GitHubProposalUncertain:
			return fmt.Errorf("GitHub did not confirm the write; check GitHub, then run `aether github proposals resolve %.12s --created <url>` or `--not-created`: %w", shown.ID, err)
		case task.GitHubProposalFailed:
			return fmt.Errorf("GitHub did not accept it: %w", err)
		}
		return fmt.Errorf("nothing was sent: %w", err)
	}
	fmt.Fprintf(out, "Published: %s\n", terminalSafe(final.Result.URL))
	return nil
}

func resolveUncertainProposal(db *store.Store, out io.Writer, p task.GitHubProposal, args []string) error {
	fs := flag.NewFlagSet("github proposals resolve", flag.ContinueOnError)
	created := fs.String("created", "", "GitHub link of the item that was created")
	notCreated := fs.Bool("not-created", false, "confirm on GitHub that nothing was created")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*created == "") == !*notCreated {
		return errors.New("usage: aether github proposals resolve <proposal-id> (--created <url> | --not-created)")
	}
	if p.State != task.GitHubProposalUncertain {
		return errors.New("only a proposal GitHub did not confirm can be resolved")
	}
	if *notCreated {
		if err := db.FailGitHubProposal(p.ID, "You confirmed on GitHub that nothing was created.", false); err != nil {
			return err
		}
		fmt.Fprintln(out, "Recorded: nothing was created.")
		return nil
	}
	if err := db.CompleteGitHubProposal(p.ID, task.GitHubProposalResult{URL: strings.TrimSpace(*created)}); err != nil {
		return err
	}
	fmt.Fprintln(out, "Recorded as published.")
	return nil
}

// terminalSafe makes untrusted text safe to print: control characters
// (including ESC sequences that could rewrite the screen) become a visible dot.
func terminalSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f):
			return '·'
		}
		return r
	}, s)
}

func stdinIsTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}
