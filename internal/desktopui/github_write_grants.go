package desktopui

import (
	"fmt"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

// Profile panel for GitHub write proposals. These grants only let the Dot
// queue proposals; sending always needs a person in the approvals tray. They
// save on their own (not with the profile form) and exist only for
// repositories the Dot can already read.

type writeGrantDraft struct {
	conversationID string
	kinds          map[int64]map[task.GitHubWriteKind]bool
	disclosure     bool
	message        string
}

var writeKindLabels = []struct {
	kind  task.GitHubWriteKind
	label string
}{
	{task.GitHubIssueComment, "Comment on issues and pull requests"},
	{task.GitHubIssueCreate, "Open issues"},
	{task.GitHubPRDraft, "Prepare draft pull requests (existing branch only)"},
}

func (a *app) loadWriteGrantDraft(conversation task.Conversation) {
	d := writeGrantDraft{conversationID: conversation.ID, kinds: map[int64]map[task.GitHubWriteKind]bool{},
		disclosure: conversation.GitHubWriteDisclosureAccepted}
	for _, grant := range conversation.GitHubWriteGrants {
		d.kinds[grant.RepositoryID] = map[task.GitHubWriteKind]bool{}
		for _, kind := range grant.Kinds {
			d.kinds[grant.RepositoryID][kind] = true
		}
	}
	a.writeGrants = d
}

func (d writeGrantDraft) grants(readable []task.GitHubRepository) []task.GitHubWriteGrant {
	var out []task.GitHubWriteGrant
	for _, repo := range readable {
		var kinds []task.GitHubWriteKind
		for _, k := range task.GitHubWriteKinds {
			if d.kinds[repo.ID][k] {
				kinds = append(kinds, k)
			}
		}
		if len(kinds) > 0 {
			out = append(out, task.GitHubWriteGrant{RepositoryID: repo.ID, Kinds: kinds})
		}
	}
	return out
}

func (a *app) githubWritePanel(c *ui.Context, theme *ui.Theme) {
	conversation, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		return
	}
	profilePanel(c, theme, "GitHub write proposals", func() {
		ui.Text(c, "Let this Dot propose comments, issues, and draft pull requests. A proposal is only text waiting in the GitHub approvals tray: nothing is sent until you review and approve it there.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		readable := make([]task.GitHubRepository, 0)
		if a.snapshot.GitHubConnected && conversation.GitHubGrantSpaceID != "" && conversation.GitHubGrantSpaceID == conversation.SpaceID && conversation.GitHubModelDisclosureAccepted {
			granted := map[int64]bool{}
			for _, id := range conversation.GitHubRepositoryIDs {
				granted[id] = true
			}
			for _, repo := range a.snapshot.GitHubRepositories {
				if repo.Selected && granted[repo.ID] {
					readable = append(readable, repo)
				}
			}
		}
		if len(readable) == 0 {
			ui.Text(c, "Save a GitHub read grant for at least one repository above first. Write proposals only exist for repositories this Dot can already read.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
			return
		}
		if a.writeGrants.conversationID != conversation.ID {
			a.loadWriteGrantDraft(conversation)
		}
		d := &a.writeGrants
		if d.kinds == nil {
			d.kinds = map[int64]map[task.GitHubWriteKind]bool{}
		}
		for _, repo := range readable[:min(len(readable), 50)] {
			ui.Text(c, repo.FullName).FontSize(11).Bold()
			if d.kinds[repo.ID] == nil {
				d.kinds[repo.ID] = map[task.GitHubWriteKind]bool{}
			}
			for _, option := range writeKindLabels {
				checked := d.kinds[repo.ID][option.kind]
				if ui.Checkbox(c.Key(fmt.Sprintf("profile-github-write-%d-%s", repo.ID, option.kind)), &checked, option.label).FillWidth().Changed() {
					d.kinds[repo.ID][option.kind] = checked
					d.message = ""
				}
			}
		}
		if len(readable) > 50 {
			ui.Text(c, "Only the first 50 readable repositories are shown.").FontSize(10).TextColor(theme.TextMuted)
		}
		wanted := d.grants(readable)
		if len(wanted) > 0 {
			ui.Text(c, "Whatever you approve is published to GitHub under your own account"+accountSuffix(a.snapshot.GitHubAccount)+
				" and may be public. Text a Dot writes can be influenced by issue and pull request content it reads, which is why you approve each item yourself.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
			if ui.Checkbox(c.Key("profile-github-write-disclosure"), &d.disclosure, "I understand approved proposals are published under my GitHub account").FillWidth().Changed() {
				d.message = ""
			}
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if d.message != "" {
				ui.Text(c, d.message).FontSize(10).TextColor(theme.TextMuted)
			}
			ui.Spacer(c)
			save := appButton(c, theme, "profile-github-write-save", "Save write permissions", buttonSecondary)
			if save.Clicked() {
				if err := a.db.SetConversationGitHubWriteGrants(conversation.ID, wanted, d.disclosure); err != nil {
					d.message = err.Error()
				} else {
					a.refresh()
					if fresh, ok := findConversation(a.snapshot, conversation.ID); ok {
						a.loadWriteGrantDraft(fresh)
					}
					a.writeGrants.message = fmt.Sprintf("Saved for %d repositories", len(wanted))
				}
			}
		})
	})
}

func accountSuffix(account string) string {
	if account == "" {
		return ""
	}
	return " (@" + account + ")"
}
