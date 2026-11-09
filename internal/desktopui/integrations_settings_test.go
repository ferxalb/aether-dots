package desktopui

import (
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"aether-dots/internal/integrations/github/ghtest"
	"github.com/egoist/mygo/ui"
)

func TestIntegrationsTabShowsGitHubComputerUseAndDisabledUpcomingProviders(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation := createTestConversation(t, a, a.selectedSpaceID, "Selected Dot", "Keep this Dot selected.")
	a.selectConversation(conversation.ID)
	ghtest.Install(t, true)
	a.githubService = github.NewService()
	a.settingsOpen = true
	tester := ui.NewTester(a.view, 1120, 760)

	if err := tester.Click("Integrations"); err != nil {
		t.Fatal(err)
	}
	texts := tester.Texts()
	for _, expected := range []string{
		"Integrations",
		"Connect the tools your Dots can work with. You're always in control of what they can access.",
		"GitHub",
		"Read repositories, issues, pull requests, and CI through your GitHub CLI login. AETHER stores no token.",
		"Connect GitHub",
		"Computer Use",
		"Apple Notes",
		"Apple Music",
		"Apple Reminders",
		"Install Cua Driver",
		"Coming soon",
		"add /Applications/CuaDriver.app and switch it on",
	} {
		if !tester.HasText(expected) {
			t.Fatalf("Integrations tab is missing %q: %q", expected, texts)
		}
	}
	if countText(texts, "Coming soon") != 1 {
		t.Fatalf("upcoming providers should have one honest status label: %q", texts)
	}
	for _, forbidden := range []string{"Connect Apple Notes", "Connect Apple Reminders", "Request permission"} {
		if tester.HasText(forbidden) {
			t.Fatalf("upcoming provider exposed a nonfunctional action %q: %q", forbidden, texts)
		}
	}
	if a.selectedConversationID != conversation.ID {
		t.Fatalf("tab navigation lost the selected Dot: %q", a.selectedConversationID)
	}
	if err := tester.Click("Skills"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Save reusable instructions and choose when a Dot should use them.") {
		t.Fatalf("switching tabs did not preserve the existing Skills section: %q", tester.Texts())
	}
	if err := tester.Click("Integrations"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Computer Use") || !tester.HasText("Apple Notes") || !tester.HasText("Apple Reminders") {
		t.Fatalf("switching back did not restore the Integrations page: %q", tester.Texts())
	}
}

func TestGitHubCLIStatesGuideTheUserAndNeverShowCredentials(t *testing.T) {
	// gh missing: explain how to get it, offer no connect button.
	ghtest.Missing(t)
	a := newMessengerTestApp(t)
	a.githubService = github.NewService()
	a.settingsOpen = true
	a.settingsTab = 2
	a.githubLoaded = false
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("GitHub CLI not found") || !tester.HasText("Get the GitHub CLI") {
		t.Fatalf("missing-gh state is not actionable: %q", tester.Texts())
	}
	if tester.HasText("Connect GitHub") {
		t.Fatalf("connect offered without a GitHub CLI: %q", tester.Texts())
	}

	// gh present but signed out: show the command to run.
	ghtest.Install(t, false)
	a.githubService = github.NewService()
	a.githubStatus = "Sign-in required"
	a.githubError = "The GitHub CLI is not signed in. Run `gh auth login` in Terminal, then press Connect."
	a.githubLoaded = true
	a.refresh()
	tester = ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Copy command") || !tester.HasText("gh auth login") || !tester.HasText("Connect GitHub") {
		t.Fatalf("signed-out state is incomplete: %q", tester.Texts())
	}

	// A rate limit on a live connection is recoverable without reconnecting.
	a.githubStatus = "Rate limited"
	a.githubError = "GitHub API rate limit reached; try again later."
	if err := a.db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	tester = ui.NewTester(a.view, 1120, 760)
	texts := tester.Texts()
	if !tester.HasText("Rate limited") || !tester.HasText(a.githubError) || !tester.HasText("Refresh GitHub access") {
		t.Fatalf("rate-limit state is not understandable or recoverable: %q", texts)
	}
	joined := strings.Join(texts, " ")
	for _, secret := range []string{"ghu_", "ghr_", "gho_", "ghp_"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("credential material appeared in UI state: %q", texts)
		}
	}
}

func TestGitHubDisconnectingBlocksNewConnectionAndDisconnectWorksWithoutGH(t *testing.T) {
	ghtest.Install(t, true)
	a := newMessengerTestApp(t)
	a.githubService = github.NewService()
	a.githubDisconnecting = true
	a.githubStatus = "Disconnecting"
	a.refreshGitHubConnection()
	a.connectGitHub()
	if a.githubStatus != "Disconnecting" || a.githubLoading {
		t.Fatalf("disconnecting state allowed a new connection flow: status=%q loading=%v", a.githubStatus, a.githubLoading)
	}

	ghtest.Missing(t)
	b := newMessengerTestApp(t)
	if err := b.db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	b.refresh()
	b.githubService = github.NewService()
	b.githubStatus = "GitHub CLI not found"
	b.githubLoaded = true
	b.settingsOpen = true
	b.settingsTab = 2
	tester := ui.NewTester(b.view, 1120, 760)
	if !tester.HasText("GitHub CLI not found") {
		t.Fatalf("missing gh not explained: %q", tester.Texts())
	}
}

func countText(texts []string, target string) int {
	count := 0
	for _, text := range texts {
		if text == target {
			count++
		}
	}
	return count
}
