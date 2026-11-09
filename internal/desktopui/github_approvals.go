package desktopui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

// GitHub approvals tray. Dots can only propose; this view is the one place a
// person turns a proposal into a GitHub write, by approving the exact text
// shown (the click passes the displayed content hash to the store).

type approvalsUI struct {
	open        bool
	editingID   string
	titleDraft  string
	bodyDraft   string
	busy        map[string]bool
	resolveURL  map[string]string
	error       string
	historyShow int
}

const approvalsHistoryStep = 10

// proposalDisplayState shows an overdue pending proposal as expired before the
// store has swept it.
func proposalDisplayState(p task.GitHubProposal, now time.Time) task.GitHubProposalState {
	if p.State == task.GitHubProposalPending && !now.Before(p.ExpiresAt) {
		return task.GitHubProposalExpired
	}
	return p.State
}

func proposalNeedsDecision(p task.GitHubProposal, now time.Time) bool {
	return proposalDisplayState(p, now) == task.GitHubProposalPending
}

func (a *app) pendingApprovalCount() int {
	now, n := time.Now(), 0
	for _, p := range a.snapshot.GitHubProposals {
		if proposalNeedsDecision(p, now) {
			n++
		}
	}
	return n
}

func (a *app) proposalRepositoryIsPublic(id int64) bool {
	for _, repo := range a.snapshot.GitHubRepositories {
		if repo.ID == id {
			return !repo.Private
		}
	}
	return false
}

func (a *app) openApprovals() {
	a.rememberCurrentConversationState()
	_ = a.db.SweepGitHubProposals()
	a.refresh()
	a.approvals.open = true
	a.approvals.error = ""
	a.activityOpen, a.settingsOpen, a.conversationDetailsOpen = false, false, false
	a.selectedDotID = ""
}

func (a *app) githubApprovalsView(c *ui.Context, theme *ui.Theme) {
	now := time.Now()
	var attention, pending, history []task.GitHubProposal
	// Newest first.
	for i := len(a.snapshot.GitHubProposals) - 1; i >= 0; i-- {
		p := a.snapshot.GitHubProposals[i]
		switch state := proposalDisplayState(p, now); {
		case state == task.GitHubProposalPending:
			pending = append(pending, p)
		case state == task.GitHubProposalUncertain || state == task.GitHubProposalExecuting:
			attention = append(attention, p)
		default:
			history = append(history, p)
		}
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).MaxWidth(860).Gap(16).Padding(24, 28, 36, 28).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "GitHub approvals").FontSize(22).Bold()
					account := a.snapshot.GitHubAccount
					line := "Dots can only propose. Nothing reaches GitHub until you approve the exact text below."
					if account != "" {
						line += " Approved items are published as @" + account + "."
					}
					ui.Text(c, line).FontSize(12).TextColor(theme.TextMuted).FillWidth()
				})
				ui.Spacer(c)
				if len(pending) > 0 {
					ui.Badge(c, itoa(len(pending))+" waiting").TextColor(theme.Accent)
				}
			})
			if !a.snapshot.GitHubConnected {
				ui.Text(c, "GitHub is not connected. Connect it in Settings → Integrations; pending proposals are cancelled while it is disconnected.").
					FontSize(11).TextColor(theme.Danger).FillWidth()
			}
			if a.approvals.error != "" {
				ui.Row(c).FillWidth().Padding(10, 11).Radius(10).Background(theme.Danger.Alpha(0.08)).Children(func() {
					ui.Text(c, a.approvals.error).FontSize(11).TextColor(theme.Danger).FillWidth()
				})
			}
			if len(attention) == 0 && len(pending) == 0 && len(history) == 0 {
				ui.Column(c).FillWidth().Gap(6).Padding(22).Radius(14).Background(theme.Surface).Border(1, theme.Border).Children(func() {
					ui.Text(c, "Nothing to approve").FontSize(14).Bold()
					ui.Text(c, "Grant a Dot write proposals in its profile, then ask it to comment on an issue, open an issue, or prepare a draft pull request. Its proposals will wait here.").
						FontSize(12).TextColor(theme.TextMuted).FillWidth()
				})
				return
			}
			a.approvalSection(c, theme, "Needs checking", "GitHub did not confirm these writes. Open GitHub, look for the item, then tell AETHER what happened.", attention, now)
			a.approvalSection(c, theme, "Waiting for your decision", "", pending, now)
			if len(history) > 0 {
				shown := a.approvals.historyShow
				if shown < approvalsHistoryStep {
					shown = approvalsHistoryStep
				}
				shown = min(shown, len(history))
				a.approvalSection(c, theme, "History", "", history[:shown], now)
				if shown < len(history) {
					if appButton(c, theme, "approvals-history-more", fmt.Sprintf("Show %d more", min(approvalsHistoryStep, len(history)-shown)), buttonSecondary).Clicked() {
						a.approvals.historyShow = shown + approvalsHistoryStep
					}
				}
			}
		})
	})
}

func (a *app) approvalSection(c *ui.Context, theme *ui.Theme, title, hint string, items []task.GitHubProposal, now time.Time) {
	if len(items) == 0 {
		return
	}
	ui.Text(c, title).FontSize(13).Bold()
	if hint != "" {
		ui.Text(c, hint).FontSize(11).TextColor(theme.TextMuted).FillWidth()
	}
	for _, p := range items {
		a.proposalCard(c, theme, p, now)
	}
}

func (a *app) proposalDotName(p task.GitHubProposal) string {
	if conversation, ok := findConversation(a.snapshot, p.ConversationID); ok {
		return conversation.Name
	}
	return "A Dot"
}

func (a *app) proposalCard(c *ui.Context, theme *ui.Theme, p task.GitHubProposal, now time.Time) {
	state := proposalDisplayState(p, now)
	busy := a.approvals.busy[p.ID]
	editing := a.approvals.editingID == p.ID && state == task.GitHubProposalPending
	ui.Column(c.Key("approval-card-"+p.ID)).FillWidth().MinWidth(0).Gap(10).Padding(15, 17).Radius(14).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, engine.ProposalHeadline(p)+" · "+p.RepositoryName).SingleLine().FontSize(13).Bold().FillWidth()
				ui.Text(c, a.proposalDotName(p)+" · proposed "+p.CreatedAt.Local().Format("Jan 2, 3:04 PM")).
					FontSize(10).TextColor(theme.TextMuted)
			})
			if state == task.GitHubProposalPending {
				ui.Text(c, "Expires in "+formatElapsed(p.ExpiresAt.Sub(now))).FontSize(10).TextColor(theme.TextMuted)
			}
			ui.Badge(c, proposalStateLabel(state)).TextColor(proposalStateColor(theme, state))
		})
		if p.Kind == task.GitHubPRDraft {
			ui.Text(c, p.Head+" → "+p.Base+" · opens as a draft").FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		if editing {
			if p.Kind != task.GitHubIssueComment {
				ui.Field(c, "Title", func() {
					ui.TextInputBase(c.Key("approval-title-"+p.ID), &a.approvals.titleDraft).FillWidth().Label("Proposal title")
				})
			}
			ui.Field(c, "Text", func() {
				ui.TextAreaBase(c.Key("approval-body-"+p.ID), &a.approvals.bodyDraft).MinHeight(120).MaxHeight(260).FillWidth().Label("Proposal text")
			})
		} else {
			if p.Title != "" {
				ui.Text(c, p.Title).FontSize(13).Bold().FillWidth()
			}
			if strings.TrimSpace(p.Body) != "" {
				ui.Scroll(c.Key("approval-body-scroll-"+p.ID)).FillWidth().MaxHeight(240).Padding(11).Radius(10).
					Background(theme.Background).Border(1, theme.Border).Children(func() {
					ui.Text(c, p.Body).FontSize(12).FillWidth()
				})
			}
		}
		if state == task.GitHubProposalPending {
			for _, warning := range engine.ProposalWarnings(p, a.proposalRepositoryIsPublic(p.RepositoryID)) {
				ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Start).Children(func() {
					ui.Icon(c, iconAlert).Size(13, 13).Shrink(0).TextColor(theme.Danger)
					ui.Text(c, warning).FontSize(11).TextColor(theme.TextMuted).FillWidth()
				})
			}
		}
		switch state {
		case task.GitHubProposalPending:
			a.pendingProposalActions(c, theme, p, editing, busy)
		case task.GitHubProposalExecuting:
			ui.Text(c, "Sending to GitHub…").FontSize(11).TextColor(theme.TextMuted)
		case task.GitHubProposalUncertain:
			a.uncertainProposalActions(c, theme, p)
		case task.GitHubProposalExecuted:
			if p.Result != nil {
				ui.Text(c, "Published: "+p.Result.URL).FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
		default:
			if p.FailureReason != "" {
				ui.Text(c, p.FailureReason).FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
		}
	})
}

func (a *app) pendingProposalActions(c *ui.Context, theme *ui.Theme, p task.GitHubProposal, editing, busy bool) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "Content "+shortApprovalHash(p.ContentHash)).FontSize(10).TextColor(theme.TextMuted)
		ui.Spacer(c)
		if editing {
			if appButton(c, theme, "approval-cancel-edit-"+p.ID, "Cancel", buttonGhost).Clicked() {
				a.approvals.editingID = ""
			}
			if appButton(c, theme, "approval-save-edit-"+p.ID, "Save changes", buttonSecondary).Disabled(busy).Clicked() {
				a.saveProposalEdit(p)
			}
			return
		}
		if appButton(c, theme, "approval-reject-"+p.ID, "Reject", buttonDanger).Disabled(busy).Clicked() {
			a.rejectProposal(p.ID)
		}
		if appButton(c, theme, "approval-edit-"+p.ID, "Edit", buttonSecondary).Disabled(busy).Clicked() {
			a.approvals.editingID, a.approvals.titleDraft, a.approvals.bodyDraft = p.ID, p.Title, p.Body
		}
		label := "Approve and send"
		if busy {
			label = "Sending…"
		}
		if appButton(c, theme, "approval-approve-"+p.ID, label, buttonPrimary).Disabled(busy || !a.snapshot.GitHubConnected).Clicked() {
			a.approveProposal(p)
		}
	})
}

func (a *app) uncertainProposalActions(c *ui.Context, theme *ui.Theme, p task.GitHubProposal) {
	if p.FailureReason != "" {
		ui.Text(c, p.FailureReason).FontSize(11).TextColor(theme.Danger).FillWidth()
	}
	if a.approvals.resolveURL == nil {
		a.approvals.resolveURL = map[string]string{}
	}
	url := a.approvals.resolveURL[p.ID]
	ui.Field(c, "If it was created, paste its GitHub link", func() {
		ui.TextInputBase(c.Key("approval-resolve-url-"+p.ID), &url).Placeholder("https://github.com/…").FillWidth().Label("Link to the created item")
	})
	a.approvals.resolveURL[p.ID] = url
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Spacer(c)
		if appButton(c, theme, "approval-notsent-"+p.ID, "It was not created", buttonSecondary).Clicked() {
			a.resolveProposal(p.ID, "")
		}
		if appButton(c, theme, "approval-sent-"+p.ID, "It was created", buttonPrimary).Disabled(strings.TrimSpace(url) == "").Clicked() {
			a.resolveProposal(p.ID, strings.TrimSpace(url))
		}
	})
}

func shortApprovalHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func proposalStateLabel(s task.GitHubProposalState) string {
	switch s {
	case task.GitHubProposalPending:
		return "Waiting"
	case task.GitHubProposalExecuting:
		return "Sending"
	case task.GitHubProposalExecuted:
		return "Published"
	case task.GitHubProposalFailed:
		return "Not sent"
	case task.GitHubProposalUncertain:
		return "Check GitHub"
	case task.GitHubProposalRejected:
		return "Rejected"
	case task.GitHubProposalExpired:
		return "Expired"
	case task.GitHubProposalCancelled:
		return "Cancelled"
	}
	return string(s)
}

func proposalStateColor(theme *ui.Theme, s task.GitHubProposalState) ui.Color {
	switch s {
	case task.GitHubProposalPending, task.GitHubProposalExecuting:
		return theme.Accent
	case task.GitHubProposalFailed, task.GitHubProposalUncertain:
		return theme.Danger
	}
	return theme.TextMuted
}

// Actions. Each one reloads the snapshot so the tray reflects the store, which
// is the source of truth shared with the CLI.

func (a *app) saveProposalEdit(p task.GitHubProposal) {
	if _, err := a.db.EditGitHubProposal(p.ID, a.approvals.titleDraft, a.approvals.bodyDraft); err != nil {
		a.approvals.error = err.Error()
		return
	}
	a.approvals.editingID, a.approvals.error = "", ""
	a.refresh()
}

func (a *app) rejectProposal(id string) {
	if err := a.db.RejectGitHubProposal(id); err != nil {
		a.approvals.error = err.Error()
	} else {
		a.approvals.error = ""
		a.adt.Info("Proposal rejected", "Nothing was sent to GitHub.")
	}
	a.refresh()
}

func (a *app) resolveProposal(id, url string) {
	var err error
	if url != "" {
		err = a.db.CompleteGitHubProposal(id, task.GitHubProposalResult{URL: url})
	} else {
		err = a.db.FailGitHubProposal(id, "You confirmed on GitHub that nothing was created.", false)
	}
	if err != nil {
		a.approvals.error = err.Error()
	} else {
		a.approvals.error = ""
		delete(a.approvals.resolveURL, id)
	}
	a.refresh()
}

// approveProposal sends the content the person is looking at (its hash). With
// a window it runs off the UI thread; without one (tests) it runs inline.
func (a *app) approveProposal(p task.GitHubProposal) {
	if a.approvals.busy == nil {
		a.approvals.busy = map[string]bool{}
	}
	if a.approvals.busy[p.ID] {
		return
	}
	a.approvals.busy[p.ID], a.approvals.error = true, ""
	db, service, win, hash := a.db, a.githubService, a.win, p.ContentHash
	finish := func(final task.GitHubProposal, err error) {
		delete(a.approvals.busy, p.ID)
		a.refresh()
		switch {
		case err == nil:
			url := ""
			if final.Result != nil {
				url = final.Result.URL
			}
			a.adt.Success("Published to GitHub", p.RepositoryName+" · "+url)
		case final.State == task.GitHubProposalUncertain:
			a.adt.Warning("GitHub did not confirm", "Check GitHub, then tell AETHER what happened in the approvals tray.")
		case final.State == task.GitHubProposalFailed:
			a.approvals.error = "GitHub did not accept it: " + err.Error()
		default:
			a.approvals.error = "Nothing was sent. " + err.Error()
		}
	}
	work := func() (task.GitHubProposal, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		return engine.ApproveGitHubProposal(ctx, db, service, p.ID, hash)
	}
	if win == nil {
		finish(work())
		return
	}
	go func() {
		final, err := work()
		win.Update(func() { finish(final, err) })
	}()
}
