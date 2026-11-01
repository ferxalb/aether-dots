package desktopui

import (
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func newSidebarTestApp(t *testing.T) *app {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local"}
	a.refresh()
	return a
}

func TestSidebarCollapseAndExpandAcrossFrames(t *testing.T) {
	a := newSidebarTestApp(t)
	tester := ui.NewTester(a.view, 900, 700)

	if err := tester.Click("Collapse sidebar"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebarCollapsed {
		t.Fatal("sidebar did not collapse")
	}
	tester.Frame()
	tester.Frame()

	if err := tester.Click("Expand sidebar"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarCollapsed {
		t.Fatal("sidebar did not expand")
	}
	tester.Frame()
	tester.Frame()
	if !tester.HasText("Recent Dots") || !tester.HasText("Settings") {
		t.Fatalf("expanded sidebar sections missing after extra frames: %q", tester.Texts())
	}
}

func TestCollapsedSidebarAddSpaceExpandsManager(t *testing.T) {
	a := newSidebarTestApp(t)
	a.sidebarCollapsed = true
	tester := ui.NewTester(a.view, 900, 700)

	if err := tester.Click("Add a space"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarCollapsed || !a.spaceManagementOpen {
		t.Fatalf("Add a space did not reveal the disclosure: collapsed=%v open=%v", a.sidebarCollapsed, a.spaceManagementOpen)
	}
	if !tester.HasText("Add a space") {
		t.Fatalf("space form title missing after expanding: %q", tester.Texts())
	}
}

func TestSidebarSelectsSessionAndStartsNewDotInSelectedSpace(t *testing.T) {
	a := newSidebarTestApp(t)
	space, err := a.db.CreateSpace("Research")
	if err != nil {
		t.Fatal(err)
	}
	dot, err := a.db.CreateDot(task.CreateDot{
		SpaceID: space.ID, Name: "Review findings", Objective: "Review the findings",
		Mode: "local", SourceRoot: "/tmp/source", OutputDir: "/tmp/output",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.selectedSpaceID = space.ID
	a.refresh()
	tester := ui.NewTester(a.view, 900, 700)

	if !tester.HasText(dot.Name) {
		t.Fatalf("selected-space history missing its session: %q", tester.Texts())
	}
	if err := tester.Click(dot.Name); err != nil {
		t.Fatal(err)
	}
	if a.selectedDotID != dot.ID {
		t.Fatalf("selected Dot = %q, want %q", a.selectedDotID, dot.ID)
	}

	if err := tester.Click("New Dot"); err != nil {
		t.Fatal(err)
	}
	if a.selectedDotID != "" || a.selectedRoutineID != "" || a.selectedSpaceID != space.ID {
		t.Fatalf("new Dot did not preserve the active Space: Dot=%q Routine=%q Space=%q", a.selectedDotID, a.selectedRoutineID, a.selectedSpaceID)
	}
}

func TestSidebarRenamesSelectedSpace(t *testing.T) {
	a := newSidebarTestApp(t)
	space, err := a.db.CreateSpace("Research")
	if err != nil {
		t.Fatal(err)
	}
	a.selectedSpaceID = space.ID
	a.refresh()
	tester := ui.NewTester(a.view, 900, 700)

	if err := tester.Click("Rename Research"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Rename space") {
		t.Fatalf("rename disclosure did not open: %q", tester.Texts())
	}
	if err := tester.Click("New space name"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("Projects")
	tester.Frame()
	if err := tester.Click("Save"); err != nil {
		t.Fatal(err)
	}

	renamed := a.db.Snapshot()
	if got := selectedSpaceName(renamed, space.ID); got != "Projects" {
		t.Fatalf("space name = %q, want Projects", got)
	}
	if a.selectedSpaceID != space.ID || a.spaceManagementOpen {
		t.Fatalf("rename changed active space or left disclosure open: Space=%q open=%v", a.selectedSpaceID, a.spaceManagementOpen)
	}
}
