package desktopui

import (
	"context"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

var (
	iconSparkles    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m12 3 1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2L12 3Z"/><path d="m19 15 .9 2.1L22 18l-2.1.9L19 21l-.9-2.1L16 18l2.1-.9L19 15Z"/></svg>`))
	iconPanelOpen   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="3"/><path d="M9 4v16M14 9l-3 3 3 3"/></svg>`))
	iconPanelClosed = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="3"/><path d="M9 4v16m3-7 3-3-3-3"/></svg>`))
	iconFolder      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H10l2 2h6.5A2.5 2.5 0 0 1 21 9.5v8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5v-10Z"/></svg>`))
	iconClock       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/></svg>`))
	iconRepeat      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m17 2 4 4-4 4"/><path d="M3 11V9a3 3 0 0 1 3-3h15M7 22l-4-4 4-4"/><path d="M21 13v2a3 3 0 0 1-3 3H3"/></svg>`))
	iconPlus        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`))
	iconCheck       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 12 4 4L19 6"/></svg>`))
	iconPlay        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="m10 8 6 4-6 4V8Z"/></svg>`))
	iconAlert       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m10.3 4.2-7.6 13A2 2 0 0 0 4.4 20h15.2a2 2 0 0 0 1.7-2.8l-7.6-13a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4m0 3h.01"/></svg>`))
	iconPause       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M10 9v6m4-6v6"/></svg>`))
	iconSettings    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="m19.4 15 .1.1a1.7 1.7 0 1 1-2.4 2.4l-.1-.1a1.7 1.7 0 0 0-2.9 1.2v.2a1.7 1.7 0 1 1-3.4 0v-.2a1.7 1.7 0 0 0-2.9-1.2l-.1.1a1.7 1.7 0 1 1-2.4-2.4l.1-.1a1.7 1.7 0 0 0-1.2-2.9H4a1.7 1.7 0 1 1 0-3.4h.2a1.7 1.7 0 0 0 1.2-2.9l-.1-.1a1.7 1.7 0 1 1 2.4-2.4l.1.1a1.7 1.7 0 0 0 2.9-1.2V2a1.7 1.7 0 1 1 3.4 0v.2a1.7 1.7 0 0 0 2.9 1.2l.1-.1a1.7 1.7 0 1 1 2.4 2.4l-.1.1a1.7 1.7 0 0 0 1.2 2.9h.2a1.7 1.7 0 1 1 0 3.4h-.2a1.7 1.7 0 0 0-1.2 2.9Z"/></svg>`))
)

type app struct {
	db       *store.Store
	win      *mygo.Window
	dataDir  string
	prefs    preferences
	snapshot task.Database

	selectedSpaceID   string
	selectedDotID     string
	selectedRoutineID string
	sidebarCollapsed  bool
	settingsOpen      bool

	objective        string
	mode             string
	sourceRoot       string
	outputDir        string
	spaceDraft       string
	routineName      string
	routineObjective string
	apiKeyDraft      string
	message          string
	live             string
	runningDotID     string
	cancelRun        context.CancelFunc

	credentialChecked bool
	credentialLoading bool
	credentialSaved   bool
	credentialError   string
	settingsSaving    bool
	modelConnection   string
}

func Run(dataDir string) error {
	db, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	prefs, err := loadPreferences(dataDir)
	if err != nil {
		return err
	}
	a := &app{db: db, dataDir: dataDir, prefs: prefs, mode: "local"}
	a.refresh()
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() {
		show := func() {
			if a.win == nil || a.win.IsDestroyed() {
				a.win = mygo.NewWindow(mygo.WindowOptions{
					Title: "AETHER Dots", Width: 1120, Height: 760, MinWidth: 760, MinHeight: 560,
					StateKey: "workspace", Content: ui.View(a.view),
					TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 44,
					Transparent: true, Vibrancy: mygo.VibrancySidebar,
					BackgroundColor: "light-dark(#f6f7fc, #10131b)",
				})
			}
			a.win.Show()
			a.win.Focus()
		}
		capture := func() {
			a.beginCapture(true)
			show()
		}
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Show AETHER Dots", Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
			{Label: "Capture a Dot", Click: func(*mygo.MenuItem, *mygo.Window) { capture() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		})
		if _, err := mygo.NewTray(mygo.TrayOptions{ID: "aether-dots", Icon: trayIcon(), IconIsTemplate: true, ToolTip: "AETHER Dots", Menu: menu}); err != nil {
			log.Printf("menu bar entry unavailable: %v", err)
			a.message = "Menu bar entry unavailable: " + err.Error()
		}
		if err := mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", capture); err != nil {
			log.Printf("capture shortcut unavailable: %v", err)
			a.message = "Capture shortcut unavailable: " + err.Error()
		}
		show()
	})
	return mygo.App.Run()
}

func (a *app) refresh() {
	a.snapshot = a.db.Snapshot()
	if len(a.snapshot.Spaces) > 0 && !containsSpace(a.snapshot, a.selectedSpaceID) {
		a.selectedSpaceID = a.snapshot.Spaces[0].ID
	}
	if a.selectedDotID != "" {
		if _, ok := findDot(a.snapshot, a.selectedDotID); !ok {
			a.selectedDotID = ""
		}
	}
}
