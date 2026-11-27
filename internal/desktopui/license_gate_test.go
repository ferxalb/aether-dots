package desktopui

import (
	"strings"
	"testing"
	"time"

	"aether-dots/internal/license"
	"aether-dots/internal/task"

	"github.com/egoist/mygo/ui"
)

func TestFreeTierLimitsAgentCreationButKeepsExistingAgents(t *testing.T) {
	a, _ := newLicenseTestApp(t, nil)
	a.settingsOpen = false
	for i := 0; i < license.FreeAgentLimit; i++ {
		a.startCreator()
	}
	if n := len(a.snapshot.Conversations); n != license.FreeAgentLimit {
		t.Fatalf("%d agents created on Free, want %d", n, license.FreeAgentLimit)
	}
	a.startCreator()
	if n := len(a.snapshot.Conversations); n != license.FreeAgentLimit {
		t.Fatalf("Free created agent %d", n)
	}
	tester := ui.NewTester(a.view, 1120, 760)
	all := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(all, "Agent limit reached") || !strings.Contains(all, "Settings → General") {
		t.Fatalf("limit not explained: %q", all)
	}
	if a.selectedConversationID == "" {
		t.Fatal("existing agents became unreachable on Free")
	}

	a.license.state = activeLicense(licenseTestNow, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC))
	a.startCreator()
	if n := len(a.snapshot.Conversations); n != license.FreeAgentLimit+1 {
		t.Fatalf("Pro could not create agent %d", license.FreeAgentLimit+1)
	}
}

func TestFreeTierRoutinesNoticeAndCreateRefused(t *testing.T) {
	a, _ := newLicenseTestApp(t, nil)
	conv := createTestConversation(t, a, a.selectedSpaceID, "Agent", "Help")
	a.refresh()
	a.settingsTab = 4
	tester := ui.NewTester(a.view, 1120, 760)
	if !strings.Contains(strings.Join(tester.Texts(), "\n"), "Routines are part of AETHER Pro") {
		t.Fatalf("routines notice missing: %q", tester.Texts())
	}
	a.routineEditorOpen = true
	a.routineDotDraft, a.routineNameDraft, a.routinePromptDraft, a.routineEveryDraft = conv.ID, "Daily", "Summarize", "24h"
	a.saveRoutine()
	if len(a.db.Snapshot().Routines) != 0 {
		t.Fatal("Free created a routine")
	}

	a.license.state = activeLicense(licenseTestNow, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC))
	a.saveRoutine()
	if len(a.db.Snapshot().Routines) != 1 {
		t.Fatal("Pro could not create a routine")
	}
	tester = ui.NewTester(a.view, 1120, 760)
	if strings.Contains(strings.Join(tester.Texts(), "\n"), "Routines are part of AETHER Pro") {
		t.Fatal("Pro still sees the Free notice")
	}
}

func TestUnlicensedBuildHasNoLimits(t *testing.T) {
	a := newMessengerTestApp(t)
	for i := 0; i < license.FreeAgentLimit+2; i++ {
		if _, err := a.db.CreateConversation(task.CreateConversation{SpaceID: a.selectedSpaceID}); err != nil {
			t.Fatal(err)
		}
	}
	a.refresh()
	before := len(a.snapshot.Conversations)
	a.startCreator()
	if len(a.snapshot.Conversations) != before+1 {
		t.Fatal("a build without licensing limited agents")
	}
}
