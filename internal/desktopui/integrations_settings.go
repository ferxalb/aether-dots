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
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const authorizedGitHubAppsURL = "https://github.com/settings/apps/authorizations"

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
	clientConfigured := a.githubService != nil && a.githubService.ClientConfigured()
	status := strings.TrimSpace(a.githubStatus)
	if status == "" {
		if !clientConfigured {
			status = "Not configured"
		} else if a.snapshot.GitHubConnected {
			status = "Refreshing credentials"
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
				ui.Text(c, githubCardBlurb(a.githubService)).
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

		if status == "Awaiting authorization" {
			ui.Column(c).FillWidth().Gap(8).Padding(12, 14).Radius(11).
				Background(theme.Background).Border(1, theme.Border).Children(func() {
				ui.Text(c, "Enter this code on GitHub").FontSize(11).Bold()
				ui.Text(c, a.githubDeviceCode).FontSize(22).Bold().LetterSpacing(1.4)
				ui.Row(c).Wrap().Gap(8).Children(func() {
					if ui.Button(c.Key("github-copy-code"), "Copy code").Padding(8, 11).Clicked() {
						c.WriteClipboard(a.githubDeviceCode)
						a.adt.Success("Code copied", "Enter the code on GitHub to authorize AETHER Dots.")
					}
					if ui.PrimaryButton(c.Key("github-open-verification"), "Open GitHub verification").Padding(8, 11).Clicked() {
						c.OpenURL(a.githubVerificationURL)
					}
					if ui.Button(c.Key("github-cancel-auth"), "Cancel").Padding(8, 11).Clicked() {
						a.cancelGitHubAuthorization()
					}
				})
				ui.Text(c, "The code expires when GitHub's authorization window ends. AETHER checks only at GitHub's requested interval.").
					FontSize(10).TextColor(theme.TextMuted).FillWidth()
			})
		}

		if !clientConfigured {
			ui.Text(c, "GitHub CLI not found. Install it from cli.github.com and run `gh auth login`, then restart AETHER.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}

		if a.snapshot.GitHubConnected && (status == "Connected" || status == "Permission required" || status == "Rate limited" || status == "Connection failed" || status == "Refreshing credentials" || status == "Session expired") {
			a.githubConnectionDetails(c, theme)
		}

		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			switch status {
			case "Awaiting authorization":
			case "Connecting", "Refreshing credentials":
				ui.Text(c, "Connecting securely…").FontSize(11).TextColor(theme.TextMuted)
				if a.snapshot.GitHubConnected {
					if ui.Button(c.Key("github-disconnect-loading"), "Disconnect").Padding(8, 12).Clicked() {
						a.disconnectGitHub()
					}
				}
			case "Disconnecting":
				ui.Text(c, "Disconnecting securely…").FontSize(11).TextColor(theme.TextMuted)
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
			case "Permission required", "Rate limited", "Connection failed":
				if clientConfigured && !a.githubLoading {
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
				if clientConfigured {
					label := "Connect GitHub"
					if status == "Session expired" {
						label = "Reconnect GitHub"
					}
					if ui.PrimaryButton(c.Key("github-connect"), label).Padding(8, 12).
						Disabled(a.githubLoading || a.githubAuthCancel != nil || a.githubDisconnecting).Clicked() {
						a.startGitHubAuthorization()
					}
				}
			}
			if a.githubAuthCancel != nil && status != "Awaiting authorization" {
				if ui.Button(c.Key("github-cancel-pending"), "Cancel connection").Padding(8, 12).Clicked() {
					a.cancelGitHubAuthorization()
				}
			}
			ui.Link(c, "GitHub authorized apps", authorizedGitHubAppsURL).FontSize(11).TextColor(theme.TextMuted)
		})

		if a.githubManageRepositories && a.snapshot.GitHubConnected {
			a.githubRepositoryManager(c, theme)
		}
		if a.snapshot.GitHubConnected {
			ui.Text(c, "Disconnecting removes AETHER's local token, repository selections, and Dot grants. It does not uninstall or revoke the GitHub App at GitHub.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
		}
	})
}

func (a *app) githubConnectionDetails(c *ui.Context, theme *ui.Theme) {
	installationCount := len(a.githubInstallations)
	repositoryCount := len(a.githubRepositories)
	selectedCount := 0
	for _, repository := range a.githubRepositories {
		if repository.Selected {
			selectedCount++
		}
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
		ui.Text(c, fmt.Sprintf("%d installation(s) · %d accessible repositories · %d selected", installationCount, repositoryCount, selectedCount)).
			FontSize(11).TextColor(theme.TextMuted)
		ui.Spacer(c)
		ui.Text(c, "Read-only").FontSize(10).Bold().TextColor(theme.Accent)
	})
	if installationCount > 0 {
		installationLabels := make([]string, 0, min(4, installationCount))
		for _, installation := range a.githubInstallations[:min(4, installationCount)] {
			label := installation.Account.Login
			if installation.Account.Type != "" {
				label += " · " + installation.Account.Type
			}
			installationLabels = append(installationLabels, label)
		}
		if installationCount > len(installationLabels) {
			installationLabels = append(installationLabels, fmt.Sprintf("and %d more", installationCount-len(installationLabels)))
		}
		ui.Text(c, "Installed for · "+strings.Join(installationLabels, ", ")).
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	}
	verifiedAt := a.snapshot.GitHubLastVerifiedAt
	if !verifiedAt.IsZero() {
		ui.Text(c, "Last successful verification · "+verifiedAt.Local().Format("Jan 2, 2006 at 3:04 PM")).
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	}
	if a.githubStatus == "Permission required" && !a.githubService.UsesCLI() {
		if installURL := githubInstallationURL(); installURL != "" {
			ui.Link(c, "Install the AETHER GitHub App", installURL).FontSize(11).TextColor(theme.Accent)
		}
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
	case "Awaiting authorization", "Connecting", "Refreshing credentials", "Disconnecting":
		color = theme.Accent
	case "Permission required", "Session expired", "Rate limited", "Connection failed":
		color = theme.Danger
	}
	ui.Box(c).Shrink(0).Padding(6, 9).Radius(10).Background(color.Alpha(0.11)).TextColor(color).
		Children(func() { ui.Text(c, status).FontSize(10).Bold() })
}

func (a *app) startGitHubRefreshIfNeeded() {
	if a.githubLoading || a.githubLoaded {
		return
	}
	if a.githubService == nil || !a.githubService.ClientConfigured() {
		a.githubStatus = "Not configured"
		a.githubError = "Install the GitHub CLI (https://cli.github.com), run `gh auth login` in Terminal, then restart AETHER."
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
	if a.githubService == nil || !a.githubService.ClientConfigured() {
		a.githubStatus = "Not configured"
		a.githubError = "Install the GitHub CLI (https://cli.github.com) and run `gh auth login`, then restart AETHER."
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
	a.githubStatus = "Refreshing credentials"
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
					ID: repository.ID, InstallationID: repository.InstallationID,
					Owner: repository.Owner.Login, Name: repository.Name, FullName: repository.FullName,
					Private: repository.Private, HTMLURL: repository.HTMLURL, DefaultBranch: repository.DefaultBranch,
				})
			}
			err = database.ReplaceGitHubRepositories(repos)
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
				if errors.Is(err, github.ErrSessionExpired) {
					a.githubError = "Your GitHub session expired. Reconnect to restore repository access."
				}
				a.refresh()
				return
			}
			a.githubUser = discovery.User
			a.githubInstallations = discovery.Installations
			a.githubRepositories = database.GitHubRepositories()
			a.githubTruncated = discovery.Truncated
			a.refresh()
			if a.githubUser.AvatarURL != "" {
				a.loadGitHubAvatar(a.githubUser.AvatarURL)
			} else {
				a.clearGitHubAvatar()
			}
			switch {
			case len(discovery.Installations) == 0 && !service.UsesCLI():
				a.githubStatus = "Permission required"
				a.githubError = "GitHub authorized AETHER, but no GitHub App installation is accessible. Install the app on an account, then refresh."
			case len(discovery.Repositories) == 0:
				a.githubStatus = "Permission required"
				a.githubError = "No repositories are accessible. Review the app installation's repository selection and read permissions."
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

func (a *app) startGitHubAuthorization() {
	if a.githubLoading || a.githubAuthCancel != nil || a.githubDisconnecting {
		return
	}
	if a.githubService == nil || !a.githubService.ClientConfigured() {
		a.githubStatus = "Not configured"
		a.githubError = "Install the GitHub CLI (https://cli.github.com) and run `gh auth login`, then restart AETHER."
		a.githubLoaded = true
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.githubAuthCancel = cancel
	a.githubStatus = "Connecting"
	a.githubError = ""
	a.githubDeviceCode = ""
	a.githubLoading = true
	win := a.win
	service := a.githubService
	database := a.db
	go func() {
		err := service.AuthorizeDevice(ctx, func(device github.DeviceAuthorization) {
			setDevice := func() {
				if a.githubDisconnecting {
					return
				}
				a.githubStatus = "Awaiting authorization"
				a.githubDeviceCode = device.UserCode
				a.githubVerificationURL = device.VerificationURI
			}
			if win != nil {
				win.Update(setDevice)
			}
			if ctx.Err() != nil {
				return
			}
			if openErr := mygo.Shell.OpenExternal(github.DeviceVerificationURL); openErr != nil && win != nil {
				win.Update(func() {
					a.githubError = "The verification page could not be opened automatically. Use Open GitHub verification to continue."
				})
			}
		})
		cancel()
		finish := func() {
			if a.githubDisconnecting {
				return
			}
			if err == nil {
				err = database.SetGitHubConnected(true)
			}
			a.githubLoading = false
			a.githubAuthCancel = nil
			a.githubDeviceCode = ""
			if err != nil {
				if errors.Is(err, context.Canceled) {
					a.githubStatus = "Disconnected"
					a.githubError = ""
				} else {
					a.githubStatus, a.githubError = githubStatusForError(err)
				}
				a.githubLoaded = true
				return
			}
			a.githubStatus = "Refreshing credentials"
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

func (a *app) cancelGitHubAuthorization() {
	if a.githubAuthCancel != nil {
		a.githubAuthCancel()
	}
}

func (a *app) disconnectGitHub() {
	if a.githubDisconnecting {
		return
	}
	if a.githubRequestCancel != nil {
		a.githubRequestCancel()
	}
	if a.githubAuthCancel != nil {
		a.githubAuthCancel()
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
		var credentialErr error
		if service != nil {
			credentialErr = service.Disconnect(context.Background())
		}
		finish := func() {
			a.githubLoading = false
			a.githubDisconnecting = false
			a.githubLoaded = true
			a.githubAuthCancel = nil
			a.githubRequestCancel = nil
			a.githubDeviceCode = ""
			a.githubUser = github.User{}
			a.githubAvatar = nil
			a.githubInstallations = nil
			a.githubRepositories = nil
			a.githubManageRepositories = false
			a.githubStatus = "Disconnected"
			a.refresh()
			if err := errors.Join(localErr, credentialErr); err != nil {
				a.githubStatus = "Connection failed"
				a.githubError = "Local GitHub access was blocked, but AETHER couldn't confirm that every local credential was removed. Try disconnecting again. " + err.Error()
				a.adt.Error("GitHub disconnect needs attention", a.githubError)
				return
			}
			a.githubError = ""
			a.adt.Push(ADTNotice{Title: "GitHub disconnected locally", Description: "AETHER removed its local token, repository selections, and Dot grants. The GitHub App may remain installed or authorized at GitHub.", Level: ADTInfo})
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
	case errors.Is(err, github.ErrInvalidClient):
		return "Not configured", "GitHub App client ID is missing or invalid for this build."
	case errors.Is(err, github.ErrAuthorizationDenied):
		return "Disconnected", "GitHub authorization was canceled. You can connect again when ready."
	case errors.Is(err, github.ErrDeviceCodeExpired):
		return "Connection failed", "The GitHub device code expired. Start a new connection."
	case errors.Is(err, github.ErrDeviceFlowDisabled):
		return "Not configured", "Device Flow is disabled for this GitHub App. Enable it in the app registration, then try again."
	case errors.Is(err, github.ErrSessionExpired):
		return "Session expired", "Your GitHub session expired. Reconnect to restore access."
	case errors.Is(err, github.ErrNotConnected):
		return "Session expired", "No GitHub credentials are available for this saved connection. Reconnect to restore access."
	case errors.Is(err, github.ErrPermissionDenied), errors.Is(err, github.ErrNotFound):
		return "Permission required", "GitHub no longer allows this read operation. Check app permissions and installation repository access."
	case errors.Is(err, github.ErrRateLimited):
		return "Rate limited", err.Error()
	default:
		return "Connection failed", err.Error()
	}
}

func githubInstallationURL() string {
	slug := github.ConfiguredAppSlug()
	if slug == "" || len(slug) > 100 {
		return ""
	}
	for _, r := range slug {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return ""
		}
	}
	return "https://github.com/apps/" + slug + "/installations/new"
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

func githubCardBlurb(service *github.Service) string {
	if service.UsesCLI() {
		return "Read repositories, issues, pull requests, and CI through your GitHub CLI login. AETHER stores no token."
	}
	return "Let your Dots inspect repositories, issues, pull requests, and CI activity."
}
