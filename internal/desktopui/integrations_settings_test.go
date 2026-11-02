package desktopui

import (
	"strings"
	"testing"

	"aether-dots/internal/integrations/github"
	"github.com/egoist/mygo/ui"
)

func TestIntegrationsTabShowsGitHubAndDisabledUpcomingProviders(t *testing.T) {
	a := newMessengerTestApp(t)
	conversation := createTestConversation(t, a, a.selectedSpaceID, "Selected Dot", "Keep this Dot selected.")
	a.selectConversation(conversation.ID)
	a.githubService = github.NewService("public-client-id", nil, nil)
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
		"Let your Dots inspect repositories, issues, pull requests, and CI activity.",
		"Connect GitHub",
		"Apple Notes",
		"Apple Reminders",
		"Coming soon",
		"AETHER does not request Apple permissions for these cards.",
	} {
		if !tester.HasText(expected) {
			t.Fatalf("Integrations tab is missing %q: %q", expected, texts)
		}
	}
	if countText(texts, "Coming soon") != 2 {
		t.Fatalf("upcoming providers should have two honest status labels: %q", texts)
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
	if !tester.HasText("Apple Notes") || !tester.HasText("Apple Reminders") {
		t.Fatalf("switching back did not restore the Integrations page: %q", tester.Texts())
	}
}

func TestGitHubAuthorizationStatesDoNotShowDuplicateConnectActions(t *testing.T) {
	a := newMessengerTestApp(t)
	a.githubService = github.NewService("public-client-id", nil, nil)
	a.settingsOpen = true
	a.settingsTab = 2
	a.githubStatus = "Awaiting authorization"
	a.githubDeviceCode = "ABCD-EFGH"
	a.githubVerificationURL = github.DeviceVerificationURL
	a.githubLoading = true
	a.githubAuthCancel = func() {}
	tester := ui.NewTester(a.view, 1120, 760)
	texts := tester.Texts()
	if !tester.HasText("Enter this code on GitHub") || !tester.HasText("ABCD-EFGH") ||
		!tester.HasText("Copy code") || !tester.HasText("Open GitHub verification") || !tester.HasText("Cancel") {
		t.Fatalf("awaiting-authorization state is incomplete: %q", texts)
	}
	if countText(texts, "Connect GitHub") != 0 {
		t.Fatalf("duplicate connect action appeared during authorization: %q", texts)
	}

	a.githubStatus = "Rate limited"
	a.githubError = "GitHub API rate limit reached; try again later."
	a.githubLoaded = true
	a.githubLoading = false
	a.githubAuthCancel = nil
	if err := a.db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	tester = ui.NewTester(a.view, 1120, 760)
	texts = tester.Texts()
	if !tester.HasText("Rate limited") || !tester.HasText(a.githubError) || !tester.HasText("Refresh GitHub access") {
		t.Fatalf("rate-limit state is not understandable or recoverable: %q", texts)
	}
	if strings.Contains(strings.Join(texts, " "), "ghu_") || strings.Contains(strings.Join(texts, " "), "ghr_") {
		t.Fatalf("credential material appeared in UI state: %q", texts)
	}
}

func TestGitHubDisconnectingBlocksNewAuthorizationAndUnconfiguredDisconnectRemainsAvailable(t *testing.T) {
	a := newMessengerTestApp(t)
	a.githubService = github.NewService("public-client-id", nil, nil)
	a.githubDisconnecting = true
	a.githubStatus = "Disconnecting"
	a.refreshGitHubConnection()
	a.startGitHubAuthorization()
	if a.githubStatus != "Disconnecting" || a.githubLoading || a.githubAuthCancel != nil {
		t.Fatalf("disconnecting state allowed a new connection flow: status=%q loading=%v auth=%v", a.githubStatus, a.githubLoading, a.githubAuthCancel != nil)
	}

	b := newMessengerTestApp(t)
	if err := b.db.SetGitHubConnected(true); err != nil {
		t.Fatal(err)
	}
	b.refresh()
	b.githubService = github.NewService("", nil, nil)
	b.githubStatus = "Not configured"
	b.githubLoaded = true
	b.settingsOpen = true
	b.settingsTab = 2
	tester := ui.NewTester(b.view, 1120, 760)
	if !tester.HasText("Not configured") || !tester.HasText("Disconnect") {
		t.Fatalf("saved GitHub connection could not be removed without the app client ID: %q", tester.Texts())
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
