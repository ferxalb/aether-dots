package desktopui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/integrations"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

const githubCLIURL = "https://cli.github.com"

func (a *app) integrationSettings(c *ui.Context, theme *ui.Theme) {
	a.startGitHubRefreshIfNeeded()
	ui.Column(c).FillWidth().MinWidth(0).Gap(14).Children(func() {
		a.mobileRemoteCard(c, theme)
		a.githubIntegrationCard(c, theme)
		a.computerUseIntegrationCard(c, theme)
		providers := integrations.InitialProviders()
		for _, provider := range providers[1:] {
			a.upcomingIntegrationCard(c, theme, "◷", provider.Name, provider.Description)
		}
		ui.Text(c, "Apple Reminders is not available yet. Computer Use needs CuaDriver enabled under System Settings → Privacy & Security → Accessibility.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
	})
}

func (a *app) githubIntegrationCard(c *ui.Context, theme *ui.Theme) {
	installed := a.githubService.Installed()
	status := strings.TrimSpace(a.githubStatus)
	if status == "" {
		if !installed {
			status = "GitHub CLI not found"
		} else if a.snapshot.GitHubConnected {
			status = "Checking GitHub CLI"
		} else {
			status = "Disconnected"
		}
	}
	card := ui.Column(c.Key("integration-github-card")).FillWidth().MinWidth(0).Gap(14).Padding(17, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			serviceMark := ui.Box(c).Size(42, 42).Shrink(0).Radius(14).
				Background(theme.Background).Border(1, theme.Border)
			serviceMark.Center().Children(func() { ui.Text(c, "◉").FontSize(21).Bold() })
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "GitHub").FontSize(15).Bold()
				ui.Text(c, "Read repositories, issues, pull requests, and CI through your GitHub CLI login. AETHER stores no token.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
			githubStatusBadge(c, theme, status)
		})

		if a.githubUser.Login != "" && a.snapshot.GitHubConnected {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(9).Children(func() {
				if a.githubAvatar != nil {
					ui.Image(c, a.githubAvatar).Size(28, 28).Fit(ui.Cover).Radius(14).Label("GitHub account avatar")
				}
				ui.Text(c, "Connected as @"+a.githubUser.Login).FontSize(12).Bold()
				ui.Spacer(c)
				if a.githubTruncated {
					ui.Text(c, "Repository list limited to the first 2,000 items").FontSize(10).TextColor(theme.TextMuted)
				}
			})
		}
		if a.githubError != "" {
			ui.Row(c).FillWidth().Gap(8).Padding(10, 11).Radius(10).
				Background(theme.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, a.githubError).FontSize(11).TextColor(theme.Danger).FillWidth()
			})
		}

		switch {
		case !installed:
			ui.Text(c, "AETHER reads GitHub through the GitHub CLI so it never holds a token. Install it, run `gh auth login` in Terminal, then restart AETHER.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			ui.Link(c, "Get the GitHub CLI", githubCLIURL).FontSize(11).TextColor(theme.Accent)
		case status == "Sign-in required":
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
				ui.Text(c, "Run this in Terminal, then press Connect:").FontSize(11).TextColor(theme.TextMuted)
				ui.Text(c, "gh auth login").FontSize(11).Bold()
				if ui.Button(c.Key("github-copy-login"), "Copy command").Padding(6, 10).Clicked() {
					c.WriteClipboard("gh auth login")
					a.adt.Success("Command copied", "Paste it in Terminal and follow the prompts.")
				}
			})
		}

		if a.snapshot.GitHubConnected && (status == "Connected" || status == "Permission required" || status == "Rate limited" || status == "Connection failed" || status == "Checking GitHub CLI" || status == "Session expired" || status == "Account changed") {
			a.githubConnectionDetails(c, theme)
		}

		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			switch status {
			case "Connecting", "Checking GitHub CLI":
				ui.Text(c, "Checking your GitHub CLI login…").FontSize(11).TextColor(theme.TextMuted)
				if a.snapshot.GitHubConnected {
					if ui.Button(c.Key("github-disconnect-loading"), "Disconnect").Padding(8, 12).Clicked() {
						a.disconnectGitHub()
					}
				}
			case "Disconnecting":
				ui.Text(c, "Disconnecting…").FontSize(11).TextColor(theme.TextMuted)
			case "Connected":
				if ui.PrimaryButton(c.Key("github-manage"), "Manage repositories").Padding(8, 12).Clicked() {
					a.githubManageRepositories = !a.githubManageRepositories
					a.githubRepositoryLimit = max(a.githubRepositoryLimit, 50)
				}
				if ui.Button(c.Key("github-refresh"), "Refresh").Padding(8, 12).Clicked() {
					a.refreshGitHubConnection()
				}
				if ui.Button(c.Key("github-disconnect"), "Disconnect").Padding(8, 12).Clicked() {
					a.disconnectGitHub()
				}
			case "GitHub CLI not found":
			case "Permission required", "Rate limited", "Connection failed":
				if !a.githubLoading {
					if ui.PrimaryButton(c.Key("github-retry"), "Refresh GitHub access").Padding(8, 12).Clicked() {
						a.refreshGitHubConnection()
					}
				}
				if ui.Button(c.Key("github-disconnect"), "Disconnect").Padding(8, 12).Clicked() {
					a.disconnectGitHub()
				}
			default:
				if a.snapshot.GitHubConnected {
					if ui.Button(c.Key("github-disconnect"), "Disconnect").Padding(8, 12).Clicked() {
						a.disconnectGitHub()
					}
				}
				label := "Connect GitHub"
				switch status {
				case "Session expired", "Account changed":
					label = "Reconnect GitHub"
				}
				if ui.PrimaryButton(c.Key("github-connect"), label).Padding(8, 12).
					Disabled(a.githubLoading || a.githubDisconnecting).Clicked() {
					a.connectGitHub()
				}
			}
		})

		if a.githubManageRepositories && a.snapshot.GitHubConnected {
			a.githubRepositoryManager(c, theme)
		}
		if a.snapshot.GitHubConnected {
			ui.Text(c, "Disconnecting removes AETHER's repository selections and Dot grants. Your GitHub CLI login is not touched; use `gh auth logout` to sign out of it.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
		}
	})
}

func (a *app) githubConnectionDetails(c *ui.Context, theme *ui.Theme) {
	repositoryCount := len(a.githubRepositories)
	selectedCount := 0
	for _, repository := range a.githubRepositories {
		if repository.Selected {
			selectedCount++
		}
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
		ui.Text(c, fmt.Sprintf("%d accessible repositories · %d selected", repositoryCount, selectedCount)).
			FontSize(11).TextColor(theme.TextMuted)
		ui.Spacer(c)
		ui.Text(c, "Read-only").FontSize(10).Bold().TextColor(theme.Accent)
	})
	verifiedAt := a.snapshot.GitHubLastVerifiedAt
	if !verifiedAt.IsZero() {
		ui.Text(c, "Last successful verification · "+verifiedAt.Local().Format("Jan 2, 2006 at 3:04 PM")).
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	}
}

func (a *app) githubRepositoryManager(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Gap(9).Padding(12, 13).Radius(11).
		Background(theme.Background).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, "Choose repositories for AETHER").FontSize(12).Bold()
				ui.Text(c, "Only selected repositories can be granted to a Dot.").FontSize(10).TextColor(theme.TextMuted)
			})
			ui.TextInput(c.Key("github-repository-search"), &a.githubRepositorySearch).
				Placeholder("Filter repositories").Width(210).Label("Filter repositories")
		})
		if len(a.githubRepositories) == 0 {
			ui.Text(c, "No accessible repositories were returned by GitHub. Check that `gh auth status` shows the right account, then Refresh.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			return
		}
		filtered := filterGitHubRepositories(a.githubRepositories, a.githubRepositorySearch)
		if len(filtered) == 0 {
			ui.Text(c, "No repositories match this filter.").FontSize(11).TextColor(theme.TextMuted)
			return
		}
		limit := a.githubRepositoryLimit
		if limit <= 0 {
			limit = 50
		}
		shown := min(len(filtered), limit)
		for _, repository := range filtered[:shown] {
			repository := repository
			checked := repository.Selected
			label := repository.FullName
			if repository.Private {
				label += " · Private"
			}
			if ui.Checkbox(c.Key("github-repository-"+fmt.Sprint(repository.ID)), &checked, label).FillWidth().Changed() {
				a.setGitHubRepositorySelected(repository.ID, checked)
			}
		}
		if shown < len(filtered) {
			if ui.Button(c.Key("github-repository-more"), fmt.Sprintf("Show %d more", min(50, len(filtered)-shown))).Padding(7, 10).Clicked() {
				a.githubRepositoryLimit = shown + 50
			}
		}
	})
}

func (a *app) upcomingIntegrationCard(c *ui.Context, theme *ui.Theme, icon, name, description string) {
	ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(12).Padding(16, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border).Children(func() {
		mark := ui.Box(c).Size(40, 40).Shrink(0).Radius(13).Background(theme.Background).Border(1, theme.Border)
		mark.Center().Children(func() { ui.Text(c, icon).FontSize(19) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, name).FontSize(14).Bold()
			ui.Text(c, description).FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		ui.Text(c, "Coming soon").FontSize(10).Bold().TextColor(theme.TextMuted)
	})
}

func githubStatusBadge(c *ui.Context, theme *ui.Theme, status string) {
	color := theme.TextMuted
	switch status {
	case "Connected":
		color = ui.Hex("#168459")
	case "Connecting", "Checking GitHub CLI", "Disconnecting":
		color = theme.Accent
	case "Permission required", "Session expired", "Rate limited", "Connection failed", "Account changed", "Sign-in required", "GitHub CLI not found":
		color = theme.Danger
	}
	ui.Box(c).Shrink(0).Padding(6, 9).Radius(10).Background(color.Alpha(0.11)).TextColor(color).
		Children(func() { ui.Text(c, status).FontSize(10).Bold() })
}

func (a *app) startGitHubRefreshIfNeeded() {
	if a.githubLoading || a.githubLoaded {
		return
	}
	if !a.githubService.Installed() {
		a.githubStatus = "GitHub CLI not found"
		a.githubError = ""
		a.githubLoaded = true
		return
	}
	if !a.snapshot.GitHubConnected {
		a.githubStatus = "Disconnected"
		a.githubError = ""
		a.githubLoaded = true
		return
	}
	a.refreshGitHubConnection()
}

func (a *app) refreshGitHubConnection() {
	if a.githubLoading || a.githubDisconnecting {
		return
	}
	if !a.githubService.Installed() {
		a.githubStatus = "GitHub CLI not found"
		a.githubError = ""
		a.githubLoaded = true
		return
	}
	if !a.snapshot.GitHubConnected {
		a.githubStatus = "Disconnected"
		a.githubError = ""
		a.githubLoaded = true
		return
	}
	a.githubLoading = true
	a.githubLoaded = false
	a.githubStatus = "Checking GitHub CLI"
	a.githubError = ""
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.githubRequestCancel = cancel
	win := a.win
	service := a.githubService
	database := a.db
	go func() {
		discovery, err := service.Discover(ctx)
		var repos []task.GitHubRepository
		if err == nil {
			repos = make([]task.GitHubRepository, 0, len(discovery.Repositories))
			for _, repository := range discovery.Repositories {
				repos = append(repos, task.GitHubRepository{
					ID:    repository.ID,
					Owner: repository.Owner.Login, Name: repository.Name, FullName: repository.FullName,
					Private: repository.Private, HTMLURL: repository.HTMLURL, DefaultBranch: repository.DefaultBranch,
				})
			}
			err = database.SetGitHubAccount(discovery.User.Login)
			if err == nil {
				err = database.ReplaceGitHubRepositories(repos)
			}
		}
		cancel()
		finish := func() {
			if a.githubDisconnecting {
				return
			}
			a.githubLoading = false
			a.githubLoaded = true
			a.githubRequestCancel = nil
			if err != nil {
				a.githubStatus, a.githubError = githubStatusForError(err)
				a.refresh()
				return
			}
			a.githubUser = discovery.User
			a.githubRepositories = database.GitHubRepositories()
			a.githubTruncated = discovery.Truncated
			a.refresh()
			if a.githubUser.AvatarURL != "" {
				a.loadGitHubAvatar(a.githubUser.AvatarURL)
			} else {
				a.clearGitHubAvatar()
			}
			switch {
			case len(discovery.Repositories) == 0:
				a.githubStatus = "Permission required"
				a.githubError = "No repositories are accessible to this GitHub CLI account. Check `gh auth status` and that you are signed in as the right user."
			default:
				a.githubStatus = "Connected"
				a.githubError = ""
			}
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) loadGitHubAvatar(avatarURL string) {
	a.clearGitHubAvatar()
	a.githubAvatarRequest++
	requestID := a.githubAvatarRequest
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	a.githubAvatarCancel = cancel
	win := a.win
	go func() {
		data, err := github.FetchAvatar(ctx, avatarURL, nil)
		var avatar ui.ImageSource
		if err == nil {
			bitmap, decodeErr := ui.DecodeBitmap(data)
			clear(data)
			if decodeErr == nil {
				avatar = bitmap
			}
		}
		wasCanceled := ctx.Err() != nil
		cancel()
		if err != nil || avatar == nil || wasCanceled {
			return
		}
		finish := func() {
			if requestID != a.githubAvatarRequest || !a.snapshot.GitHubConnected || a.githubUser.AvatarURL != avatarURL {
				return
			}
			a.githubAvatarCancel = nil
			a.githubAvatar = avatar
			a.refresh()
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) clearGitHubAvatar() {
	if a.githubAvatarCancel != nil {
		a.githubAvatarCancel()
		a.githubAvatarCancel = nil
	}
	a.githubAvatarRequest++
	a.githubAvatar = nil
}

// connectGitHub confirms the GitHub CLI is signed in, records which account it
// is, and loads that account's repositories. There is no code or token to enter.
func (a *app) connectGitHub() {
	if a.githubLoading || a.githubDisconnecting {
		return
	}
	if !a.githubService.Installed() {
		a.githubStatus = "GitHub CLI not found"
		a.githubError = ""
		a.githubLoaded = true
		return
	}
	a.githubStatus = "Connecting"
	a.githubError = ""
	a.githubLoading = true
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	a.githubRequestCancel = cancel
	win := a.win
	service := a.githubService
	database := a.db
	go func() {
		user, err := service.Connect(ctx)
		if err == nil {
			err = database.SetGitHubConnected(true)
		}
		if err == nil {
			err = database.SetGitHubAccount(user.Login)
		}
		cancel()
		finish := func() {
			if a.githubDisconnecting {
				return
			}
			a.githubLoading = false
			a.githubRequestCancel = nil
			if err != nil {
				a.githubStatus, a.githubError = githubStatusForError(err)
				a.githubLoaded = true
				return
			}
			a.githubStatus = "Checking GitHub CLI"
			a.githubLoaded = false
			a.refresh()
			a.refreshGitHubConnection()
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) disconnectGitHub() {
	if a.githubDisconnecting {
		return
	}
	if a.githubRequestCancel != nil {
		a.githubRequestCancel()
	}
	a.clearGitHubAvatar()
	a.githubLoading = true
	a.githubDisconnecting = true
	a.githubStatus = "Disconnecting"
	a.githubError = "Disconnecting from GitHub…"
	if a.githubService != nil {
		a.githubService.CancelActiveRequests()
	}
	win := a.win
	service := a.githubService
	database := a.db
	go func() {
		localErr := database.DisconnectGitHub()
		credentialErr := service.Disconnect(context.Background())
		finish := func() {
			a.githubLoading = false
			a.githubDisconnecting = false
			a.githubLoaded = true
			a.githubRequestCancel = nil
			a.githubUser = github.User{}
			a.githubAvatar = nil
			a.githubRepositories = nil
			a.githubManageRepositories = false
			a.githubStatus = "Disconnected"
			a.refresh()
			if err := errors.Join(localErr, credentialErr); err != nil {
				a.githubStatus = "Connection failed"
				a.githubError = "AETHER couldn't confirm that GitHub access was fully removed. Try disconnecting again. " + err.Error()
				a.adt.Error("GitHub disconnect needs attention", a.githubError)
				return
			}
			a.githubError = ""
			a.adt.Push(ADTNotice{Title: "GitHub disconnected locally", Description: "AETHER removed its repository selections and Dot grants. Your GitHub CLI login is untouched.", Level: ADTInfo})
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) setGitHubRepositorySelected(repositoryID int64, selected bool) {
	ids := make([]int64, 0, len(a.githubRepositories))
	for _, repository := range a.githubRepositories {
		if repository.ID == repositoryID {
			if selected {
				ids = append(ids, repository.ID)
			}
		} else if repository.Selected {
			ids = append(ids, repository.ID)
		}
	}
	if err := a.db.SetGitHubRepositorySelection(ids); err != nil {
		a.adt.Error("Couldn't update repository selection", err.Error())
		return
	}
	a.githubRepositories = a.db.GitHubRepositories()
	a.githubStatus = "Connected"
	a.refresh()
}

func githubStatusForError(err error) (string, string) {
	switch {
	case errors.Is(err, github.ErrCLINotInstalled):
		return "GitHub CLI not found", ""
	case errors.Is(err, github.ErrCLINotAuthenticated):
		return "Sign-in required", "The GitHub CLI is not signed in. Run `gh auth login` in Terminal, then press Connect."
	case errors.Is(err, github.ErrAccountChanged):
		return "Account changed", err.Error()
	case errors.Is(err, github.ErrSessionExpired):
		return "Session expired", "Your GitHub CLI session expired. Run `gh auth login`, then reconnect."
	case errors.Is(err, github.ErrNotConnected):
		return "Disconnected", "GitHub is disconnected in AETHER. Press Connect to use your GitHub CLI login."
	case errors.Is(err, github.ErrPermissionDenied), errors.Is(err, github.ErrNotFound):
		return "Permission required", "GitHub no longer allows this read operation for the GitHub CLI account. Check `gh auth status` and the account's access."
	case errors.Is(err, github.ErrRateLimited):
		return "Rate limited", err.Error()
	default:
		return "Connection failed", err.Error()
	}
}

func filterGitHubRepositories(repositories []task.GitHubRepository, query string) []task.GitHubRepository {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return repositories
	}
	filtered := make([]task.GitHubRepository, 0)
	for _, repository := range repositories {
		if strings.Contains(strings.ToLower(repository.FullName), query) {
			filtered = append(filtered, repository)
		}
	}
	return filtered
}
