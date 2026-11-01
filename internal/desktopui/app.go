package desktopui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"aether-dots/internal/engine"
	"aether-dots/internal/model"
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
)

type app struct {
	db  *store.Store
	win *mygo.Window

	snapshot          task.Database
	selectedSpaceID   string
	selectedDotID     string
	selectedRoutineID string
	sidebarCollapsed  bool

	objective        string
	mode             string
	sourceRoot       string
	outputDir        string
	spaceDraft       string
	routineName      string
	routineObjective string
	message          string
	live             string
	runningDotID     string
	cancelRun        context.CancelFunc
}

func Run(dataDir string) error {
	db, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	a := &app{db: db, mode: "local"}
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

func (a *app) theme(c *ui.Context) *ui.Theme {
	theme := *c.Theme()
	if theme.Dark {
		theme.Background = ui.Hex("#11141c")
		theme.Surface = ui.Hex("#1b202b")
		theme.SurfaceHover = ui.Hex("#252b39")
		theme.SurfacePressed = ui.Hex("#303748")
		theme.Border = ui.Hex("#343c4e")
		theme.Text = ui.Hex("#f1f2fb")
		theme.TextMuted = ui.Hex("#a3a9bd")
		theme.Accent = ui.Hex("#a294ff")
		theme.AccentHover = ui.Hex("#b4aaff")
		theme.AccentPressed = ui.Hex("#8a79ed")
		theme.AccentText = ui.Hex("#17142b")
		theme.Selection = ui.RGBA(162, 148, 255, 0.30)
		theme.Focus = ui.RGBA(180, 170, 255, 0.62)
		theme.Scrollbar = ui.RGBA(220, 222, 240, 0.34)
	} else {
		theme.Background = ui.Hex("#f5f6fb")
		theme.Surface = ui.Hex("#ffffff")
		theme.SurfaceHover = ui.Hex("#eeeff8")
		theme.SurfacePressed = ui.Hex("#e4e5f1")
		theme.Border = ui.Hex("#e0e2ed")
		theme.Text = ui.Hex("#222438")
		theme.TextMuted = ui.Hex("#6e7389")
		theme.Accent = ui.Hex("#6557d9")
		theme.AccentHover = ui.Hex("#5749c8")
		theme.AccentPressed = ui.Hex("#493eaf")
		theme.AccentText = ui.Hex("#ffffff")
		theme.Selection = ui.RGBA(101, 87, 217, 0.20)
		theme.Focus = ui.RGBA(101, 87, 217, 0.50)
		theme.Scrollbar = ui.RGBA(55, 57, 79, 0.30)
	}
	if c.Preferences().HighContrast {
		theme.Border = theme.Border.Mix(theme.Text, 0.48)
		theme.TextMuted = theme.TextMuted.Mix(theme.Text, 0.4)
		theme.Focus.A = 255
		theme.Scrollbar = theme.Scrollbar.Alpha(1.6)
	}
	theme.Radius = 10
	theme.Spacing = 4
	c.SetTheme(&theme)
	return c.Theme()
}

func sidebarColor(theme *ui.Theme, vibrant bool) ui.Color {
	if vibrant {
		return theme.Surface.Alpha(0.76)
	}
	return theme.Surface
}

func paneColor(theme *ui.Theme, vibrant bool) ui.Color {
	if vibrant {
		return theme.Background.Alpha(0.94)
	}
	return theme.Background
}

func sidebarSection(c *ui.Context, theme *ui.Theme, icon *ui.SVG, label string) {
	ui.Icon(c, icon).Size(14, 14).TextColor(theme.TextMuted)
	ui.Text(c, label).FontSize(10).Bold().TextColor(theme.TextMuted)
}

func styleSidebarItem(button ui.Element, theme *ui.Theme, selected bool) {
	button.Background(ui.Transparent).Border(1, ui.Transparent).TextColor(theme.Text)
	if selected {
		button.Background(theme.Accent.Alpha(0.13)).Border(1, theme.Accent.Alpha(0.20)).TextColor(theme.Accent)
	} else if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
}

func statusIcon(status task.Status) *ui.SVG {
	switch status {
	case task.Done:
		return iconCheck
	case task.Running:
		return iconPlay
	case task.Failed:
		return iconAlert
	case task.Paused:
		return iconPause
	case task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return iconClock
	default:
		return iconPlay
	}
}

func statusColor(theme *ui.Theme, status task.Status) ui.Color {
	switch status {
	case task.Done:
		return theme.Success
	case task.Running:
		return theme.Accent
	case task.Failed:
		return theme.Danger
	case task.Paused, task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return theme.Warning
	default:
		return theme.TextMuted
	}
}

func (a *app) view(c *ui.Context) {
	theme := a.theme(c)
	bar := c.TitleBar()
	dragHeight := max(bar.Height, 32)
	contentTop := max(dragHeight+14, 48)
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	rootColor := theme.Background
	if vibrant {
		rootColor = ui.Transparent
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(rootColor).Children(func() {
		a.sidebar(c, theme, contentTop, vibrant)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.55))
		pane := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Margin(8).Radius(20).
			Background(paneColor(theme, vibrant)).Border(1, theme.Border.Alpha(0.55)).
			Shadow(0, 4, 24, 0, ui.RGBA(0, 0, 0, 0.12))
		pane.Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Gap(20).Padding(contentTop, 28, 28, 28).Children(func() {
					a.header(c, theme)
					if a.selectedRoutineID != "" {
						a.routineView(c, theme)
					} else if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
						a.dotView(c, theme, dot)
					} else {
						a.captureView(c, theme)
					}
				})
			})
		})
		// The hidden native title bar keeps the traffic lights and resize
		// affordances. This clear strip remains a direct window drag target.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(dragHeight).DragWindow()
	})
}

func (a *app) sidebar(c *ui.Context, theme *ui.Theme, top float32, vibrant bool) {
	width, inset := float32(248), float32(16)
	if a.sidebarCollapsed {
		width, inset = 72, 9
	}
	panel := ui.Column(c).Width(width).FillHeight().Gap(14).Padding(top, inset, 14, inset).
		Background(sidebarColor(theme, vibrant))
	panel.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			brand := ui.Box(c).Size(30, 30).Shrink(0).Radius(10).
				Gradient(theme.Accent, ui.Hex("#45b8c8"), 45).TextColor(ui.Hex("#ffffff"))
			brand.Children(func() { ui.Icon(c, iconSparkles).Size(17, 17) })
			if !a.sidebarCollapsed {
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, "AETHER").FontSize(14).Bold()
					ui.Text(c, "DOTS").FontSize(9).Bold().TextColor(theme.TextMuted)
				})
			}
		})

		if !a.sidebarCollapsed {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 4).Children(func() {
				sidebarSection(c, theme, iconFolder, "SPACES")
			})
		}
		for _, space := range a.snapshot.Spaces {
			button := ui.Button(c.Key("space-"+space.ID), "").FillWidth().Height(36).
				Padding(8, 10).Gap(9).Radius(10).Label(space.Name).Tooltip(space.Name)
			styleSidebarItem(button, theme, space.ID == a.selectedSpaceID)
			button.Children(func() {
				ui.Icon(c, iconFolder).Size(16, 16)
				if !a.sidebarCollapsed {
					ui.Text(c, space.Name).SingleLine().FontSize(13)
				}
			})
			if button.Clicked() {
				a.selectedSpaceID = space.ID
				a.selectedDotID = ""
				a.selectedRoutineID = ""
				a.refresh()
			}
		}
		if !a.sidebarCollapsed {
			ui.Row(c).Gap(6).Children(func() {
				ui.TextInput(c.Key("sidebar-space-draft"), &a.spaceDraft).Placeholder("Space name").Grow(1).MinWidth(0).Label("Space name")
				add := ui.Button(c.Key("sidebar-add-space"), "Add").Label("Add space")
				if add.Clicked() {
					space, err := a.db.CreateSpace(a.spaceDraft)
					if err != nil {
						a.message = err.Error()
					} else {
						a.selectedSpaceID = space.ID
						a.spaceDraft = ""
						a.message = "Space saved."
					}
					a.refresh()
				}
			})
		} else {
			add := ui.Button(c.Key("add-space"), "").FillWidth().Height(36).
				Label("Add space").Tooltip("Add space").Padding(8).Radius(10)
			add.Children(func() { ui.Icon(c, iconPlus).Size(16, 16) })
			if add.Clicked() {
				a.sidebarCollapsed = false
			}
		}

		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if !a.sidebarCollapsed {
				sidebarSection(c, theme, iconClock, "RECENT DOTS")
			} else {
				ui.Icon(c, iconClock).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Recent Dots")
			}
			ui.Spacer(c)
			newButton := ui.Button(c.Key("new-dot"), "").Height(30).Padding(5, 8).Radius(9).
				Label("New Dot").Tooltip("New Dot")
			if !a.sidebarCollapsed {
				newButton.Children(func() {
					ui.Icon(c, iconPlus).Size(15, 15)
					ui.Text(c, "New").FontSize(12)
				})
			} else {
				newButton.Width(30)
				newButton.Children(func() { ui.Icon(c, iconPlus).Size(16, 16) })
			}
			if newButton.Clicked() {
				a.beginCapture(false)
			}
		})

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				for i := len(a.snapshot.Dots) - 1; i >= 0; i-- {
					dot := a.snapshot.Dots[i]
					selected := dot.ID == a.selectedDotID
					button := ui.Button(c.Key("dot-"+dot.ID), "").FillWidth().Height(36).
						Padding(8, 10).Gap(9).Radius(10).Label(dot.Name).Tooltip(dot.Name + " · " + humanStatus(dot.Status))
					styleSidebarItem(button, theme, selected)
					button.Children(func() {
						ui.Icon(c, statusIcon(dot.Status)).Size(16, 16).TextColor(statusColor(theme, dot.Status))
						if !a.sidebarCollapsed {
							ui.Text(c, dot.Name).SingleLine().FontSize(12)
						}
					})
					if button.Clicked() {
						a.selectedDotID = dot.ID
						a.selectedRoutineID = ""
						a.selectedSpaceID = dot.SpaceID
						a.refresh()
					}
				}
				if len(a.snapshot.Routines) > 0 {
					ui.Divider(c)
					if !a.sidebarCollapsed {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 4).Children(func() {
							sidebarSection(c, theme, iconRepeat, "ROUTINES")
						})
					} else {
						ui.Icon(c, iconRepeat).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Routines")
					}
					for _, routine := range a.snapshot.Routines {
						selected := routine.ID == a.selectedRoutineID
						button := ui.Button(c.Key("routine-"+routine.ID), "").FillWidth().Height(36).
							Padding(8, 10).Gap(9).Radius(10).Label(routine.Name).Tooltip(routine.Name)
						styleSidebarItem(button, theme, selected)
						button.Children(func() {
							ui.Icon(c, iconRepeat).Size(16, 16)
							if !a.sidebarCollapsed {
								ui.Text(c, routine.Name).SingleLine().FontSize(12)
							}
						})
						if button.Clicked() {
							a.selectedRoutineID = routine.ID
							a.selectedDotID = ""
							a.selectedSpaceID = ""
							a.routineName = routine.Name
							a.routineObjective = routine.Objective
							a.refresh()
						}
					}
				}
			})
		})

		if a.sidebarCollapsed {
			ui.Icon(c, iconSparkles).Size(16, 16).TextColor(theme.TextMuted).Tooltip("Local runtime · this device")
		} else {
			ui.Text(c, "Local runtime · this device").FontSize(11).TextColor(theme.TextMuted)
		}
	})
}

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		toggle := ui.Button(c.Key("toggle-sidebar"), "").Size(36, 36).Padding(8).Radius(10).
			Label("Toggle sidebar").Tooltip("Toggle sidebar")
		icon := iconPanelOpen
		label := "Collapse sidebar"
		if a.sidebarCollapsed {
			icon, label = iconPanelClosed, "Expand sidebar"
		}
		toggle.Label(label).Tooltip(label)
		if toggle.Hovered() {
			toggle.Background(theme.SurfaceHover)
		}
		toggle.Children(func() { ui.Icon(c, icon).Size(18, 18) })
		if toggle.Clicked() {
			a.sidebarCollapsed = !a.sidebarCollapsed
		}
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "AETHER Dots").FontSize(19).Bold()
			ui.Text(c, "One outcome. One permission scope. One resumable history.").FontSize(12).TextColor(theme.TextMuted)
		})
		ui.Spacer(c)
		if a.selectedRoutineID != "" || a.selectedDotID != "" {
			newCapture := ui.Button(c.Key("header-new-dot"), "").Padding(9, 12).Gap(7).
				Label("Create a new Dot").Tooltip("Create a new Dot")
			newCapture.Children(func() {
				ui.Icon(c, iconPlus).Size(15, 15)
				ui.Text(c, "New Dot").FontSize(12)
			})
			if newCapture.Clicked() {
				a.beginCapture(false)
			}
		}
		ui.Badge(c, "⌘ ⇧ SPACE · CAPTURE")
	})
	if a.message != "" {
		status := ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(10, 12).Radius(11).
			Background(theme.Accent.Alpha(0.1)).Border(1, theme.Accent.Alpha(0.18))
		status.Children(func() {
			ui.Icon(c, iconSparkles).Size(15, 15).TextColor(theme.Accent)
			ui.Text(c, a.message).FontSize(12).TextColor(theme.Text).FillWidth()
		})
	}
}

func (a *app) captureView(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Text(c, "Capture a task").FontSize(30).Bold()
		ui.Text(c, "Choose exactly what AETHER may read and where its local result may be saved.").TextColor(theme.TextMuted)
		ui.Field(c, "Outcome", func() {
			ui.TextArea(c, &a.objective).Placeholder("Find open design decisions in these notes and prepare a concise brief with source evidence.").MinHeight(104).FillWidth()
		})
		ui.Field(c, "Execution", func() {
			ui.Select(c, &a.mode, []string{"local", "model"}).Width(220)
			if a.mode == "model" {
				cfg := model.FromEnvironment()
				ui.Text(c, fmt.Sprintf("Selected text is sent to %s using %s (%s). API key comes from %s; it is not saved.", cfg.EndpointLabel(), cfg.API, cfg.Model, cfg.KeyEnv)).TextColor(theme.TextMuted)
			} else {
				ui.Text(c, "Local extraction uses a fixed list of open-decision markers. It does not infer or group decisions.").TextColor(theme.TextMuted)
			}
		})
		ui.Row(c).Gap(12).Children(func() {
			folderCard(c, theme, iconFolder, "Source folder", a.sourceRoot, "Choose source…", func() { a.pickFolder(true) })
			folderCard(c, theme, iconFolder, "Output folder", a.outputDir, "Choose destination…", func() { a.pickFolder(false) })
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			create := ui.PrimaryButton(c.Key("create-dot"), "").Gap(8).Padding(10, 16).
				Label("Create Dot").Disabled(strings.TrimSpace(a.objective) == "" || a.sourceRoot == "" || a.outputDir == "")
			create.Children(func() {
				ui.Icon(c, iconPlus).Size(16, 16)
				ui.Text(c, "Create Dot").FontSize(13).Bold()
			})
			if create.Clicked() {
				a.createDot()
			}
			ui.Text(c, "Space: "+selectedSpaceName(a.snapshot, a.selectedSpaceID)).TextColor(theme.TextMuted)
			ui.Spacer(c)
			if ui.Button(c, "Rename Space").Clicked() {
				if err := a.db.RenameSpace(a.selectedSpaceID, a.spaceDraft); err != nil {
					a.message = err.Error()
				} else {
					a.spaceDraft = ""
					a.message = "Space renamed."
				}
				a.refresh()
			}
		})
		ui.Divider(c)
		a.modelStatus(c, theme)
	})
}

func (a *app) dotView(c *ui.Context, theme *ui.Theme, dot task.Dot) {
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, dot.Name).FontSize(27).Bold()
				ui.Text(c, "Space: "+selectedSpaceName(a.snapshot, dot.SpaceID)+"  ·  "+humanStatus(dot.Status)).TextColor(theme.TextMuted)
			})
			ui.Spacer(c)
			if dot.Status == task.Running && a.runningDotID == dot.ID {
				if ui.Button(c, "Pause").Clicked() && a.cancelRun != nil {
					a.cancelRun()
					a.message = "Pause requested; the run will stop at its next safe boundary."
				}
			} else if dot.Status != task.Done && dot.Status != task.WaitingApproval && dot.Status != task.WaitingDevice {
				label := "Run"
				if dot.Status == task.Paused || dot.Status == task.Failed || dot.Status == task.WaitingInput {
					label = "Resume"
				}
				if ui.PrimaryButton(c, label).Clicked() {
					a.startRun(dot.ID)
				}
			}
		})
		ui.Text(c, dot.Objective).FontSize(16).FillWidth()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Switch to local extraction").Disabled(dot.Mode == "local" || dot.Status == task.Running).Clicked() {
				if err := a.db.SetMode(dot.ID, "local"); err != nil {
					a.message = err.Error()
				} else {
					a.message = "Mode set to local extraction; no source text will be sent to a model."
				}
				a.refresh()
			}
			if ui.Button(c, "Test model").Clicked() {
				a.testModel()
			}
		})
		if dot.Status == task.Running && a.live != "" {
			ui.Fieldset(c, "Live model draft", func() { ui.Text(c, tail(a.live, 1800)).FillWidth() })
		}
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "EVENT TRAIL").FontSize(11).Bold()
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d events", len(dot.Events))).FontSize(11).TextColor(theme.TextMuted)
		})
		ui.Scroll(c).Height(230).FillWidth().Children(func() {
			ui.Column(c).Gap(8).FillWidth().Children(func() {
				start := len(dot.Events) - 18
				if start < 0 {
					start = 0
				}
				for _, event := range dot.Events[start:] {
					ui.Row(c).AlignItems(ui.Start).Gap(10).Children(func() {
						ui.Text(c, fmt.Sprintf("%02d", event.Sequence)).FontSize(11).TextColor(theme.TextMuted).Width(24)
						ui.Column(c).Gap(2).FillWidth().Children(func() {
							ui.Text(c, event.Type).FontSize(12).Bold()
							ui.Text(c, event.Message).FontSize(12).FillWidth()
						})
					})
				}
			})
		})
		ui.Fieldset(c, "Reach and permissions", func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, "Read · local only · "+dot.SourceRoot).FillWidth()
				ui.Text(c, "Write · local only · "+dot.OutputDir).FillWidth()
				if dot.Mode == "model" {
					cfg := model.FromEnvironment()
					ui.Text(c, "Model · selected text is sent to "+cfg.EndpointLabel()+" ("+cfg.API+", "+cfg.Model+")").TextColor(theme.TextMuted)
				}
				if dot.Checkpoint != "" {
					ui.Text(c, "Checkpoint · "+tail(dot.Checkpoint, 160)).TextColor(theme.TextMuted)
				}
			})
		})
		if len(dot.Artifacts) > 0 {
			ui.Fieldset(c, "Verified artifact", func() {
				for _, artifact := range dot.Artifacts {
					ui.Text(c, artifact.Path).FillWidth()
					ui.Text(c, fmt.Sprintf("SHA-256 %s · %d bytes", artifact.SHA256, artifact.Bytes)).FontSize(11).TextColor(theme.TextMuted)
				}
			})
		}
		if dot.Status == task.Done && dot.RoutineID == "" {
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Save as Routine…").Clicked() {
					routine, err := a.db.PromoteRoutine(dot.ID, a.routineName)
					if err != nil {
						a.message = err.Error()
					} else {
						a.selectedRoutineID = routine.ID
						a.selectedDotID = ""
						a.routineName = routine.Name
						a.routineObjective = routine.Objective
						a.message = "Routine saved with the same local grants."
					}
					a.refresh()
				}
				ui.TextInput(c, &a.routineName).Placeholder("Routine name").Width(220)
			})
		}
	})
}

func (a *app) routineView(c *ui.Context, theme *ui.Theme) {
	var selected *task.Routine
	for i := range a.snapshot.Routines {
		if a.snapshot.Routines[i].ID == a.selectedRoutineID {
			selected = &a.snapshot.Routines[i]
			break
		}
	}
	if selected == nil {
		return
	}
	ui.Column(c).FillWidth().Gap(15).Children(func() {
		ui.Text(c, "Editable Routine").FontSize(28).Bold()
		ui.Text(c, fmt.Sprintf("Version %d · promoted from Dot %s", selected.Version, selected.SourceDotID)).TextColor(theme.TextMuted)
		ui.Field(c, "Name", func() { ui.TextInput(c, &a.routineName).FillWidth() })
		ui.Field(c, "Objective", func() { ui.TextArea(c, &a.routineObjective).MinHeight(120).FillWidth() })
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Save Routine").Clicked() {
				if err := a.db.EditRoutine(selected.ID, a.routineName, a.routineObjective); err != nil {
					a.message = err.Error()
				} else {
					a.message = "Routine updated. Its grants remain unchanged."
				}
				a.refresh()
			}
			if ui.Button(c, "Run Routine").Clicked() {
				dot, err := a.db.DotFromRoutine(selected.ID, a.snapshot.Spaces[0].ID)
				if err != nil {
					a.message = err.Error()
				} else {
					a.selectedRoutineID = ""
					a.selectedDotID = dot.ID
					a.message = "New Dot captured with this Routine's original source and output grants."
				}
				a.refresh()
			}
		})
		ui.Fieldset(c, "Fixed local permission envelope", func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "Read only: "+selected.SourceRoot).FillWidth()
				ui.Text(c, "Write only: "+selected.OutputDir).FillWidth()
				ui.Text(c, "Precondition: "+selected.Precondition).FillWidth()
				ui.Text(c, "Verification: "+selected.Verification).FillWidth()
			})
		})
	})
}

func (a *app) modelStatus(c *ui.Context, theme *ui.Theme) {
	cfg := model.FromEnvironment()
	card := ui.Column(c).FillWidth().Gap(12).Padding(16).Radius(14).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
			ui.Icon(c, iconSparkles).Size(17, 17).TextColor(theme.Accent)
			ui.Text(c, "OpenAI-compatible model").FontSize(14).Bold()
			ui.Spacer(c)
			state := "Key missing"
			stateColor := theme.Warning
			if cfg.APIKey != "" {
				state, stateColor = "Key available", theme.Success
			}
			ui.Badge(c, state).TextColor(stateColor)
		})
		ui.Text(c, fmt.Sprintf("Endpoint: %s  ·  API: %s  ·  model: %s", cfg.EndpointLabel(), cfg.API, cfg.Model)).FontSize(12).FillWidth()
		keyState := "missing"
		if cfg.APIKey != "" {
			keyState = "available in process environment"
		}
		ui.Text(c, fmt.Sprintf("Credential: %s (%s)", cfg.KeyEnv, keyState)).FontSize(12).TextColor(theme.TextMuted)
		ui.Text(c, "Configure AETHER_MODEL_BASE_URL, AETHER_MODEL_NAME, AETHER_MODEL_API, AETHER_MODEL_API_KEY_ENV, and the named key variable before model runs.").FontSize(12).TextColor(theme.TextMuted).FillWidth()
		check := ui.Button(c.Key("test-model-connection"), "").FillWidth().Padding(9, 12).Gap(8).Radius(9).
			Label("Test model connection")
		check.Children(func() {
			ui.Icon(c, iconPlay).Size(15, 15).TextColor(theme.Accent)
			ui.Text(c, "Test model connection").FontSize(12)
		})
		if check.Clicked() {
			a.testModel()
		}
	})
}

func (a *app) pickFolder(sourceFolder bool) {
	win := a.win
	go func() {
		title := "Choose output folder"
		if sourceFolder {
			title = "Choose source folder"
		}
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: true})
		if err == nil && len(paths) == 0 {
			return
		}
		if win != nil {
			win.Update(func() {
				if err != nil {
					a.message = err.Error()
					return
				}
				if sourceFolder {
					a.sourceRoot = paths[0]
				} else {
					a.outputDir = paths[0]
				}
				a.message = "Folder selected. AETHER will use only the displayed local scope."
			})
		}
	}()
}

func (a *app) createDot() {
	dot, err := a.db.CreateDot(task.CreateDot{
		SpaceID: a.selectedSpaceID, Name: "", Objective: a.objective,
		Mode: a.mode, SourceRoot: a.sourceRoot, OutputDir: a.outputDir,
	})
	if err != nil {
		a.message = err.Error()
		return
	}
	a.selectedDotID = dot.ID
	a.selectedRoutineID = ""
	a.message = "Dot captured and durably saved."
	a.refresh()
}

func (a *app) beginCapture(preserveScope bool) {
	a.selectedDotID, a.selectedRoutineID = "", ""
	a.objective = ""
	a.mode = "local"
	if !preserveScope {
		a.sourceRoot, a.outputDir = "", ""
	}
}

func (a *app) startRun(id string) {
	if a.cancelRun != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.runningDotID = id
	a.live = ""
	a.message = "Run started; events are saved as work progresses."
	win := a.win
	go func() {
		generator := model.Generator{Config: model.FromEnvironment()}
		err := engine.Run(ctx, a.db, id, &generator, func(delta string) {
			if win != nil {
				win.Update(func() { a.live = tail(a.live+delta, 6000) })
			}
		})
		if win != nil {
			win.Update(func() {
				a.cancelRun = nil
				a.runningDotID = ""
				a.refresh()
				if err != nil && !errorsIsCanceled(err) {
					a.message = err.Error()
				} else if err == nil {
					a.message = "Run finished; the local artifact was byte-verified."
				}
			})
		}
	}()
}

func (a *app) testModel() {
	if a.cancelRun != nil {
		a.message = "A task is already running."
		return
	}
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := (model.Generator{Config: model.FromEnvironment()}).Test(ctx)
		if win != nil {
			win.Update(func() {
				if err != nil {
					a.message = "Model test failed: " + err.Error()
				} else {
					a.message = "Model test passed; response marker received."
				}
			})
		}
	}()
}

func folderCard(c *ui.Context, theme *ui.Theme, icon *ui.SVG, title, path, button string, action func()) {
	card := ui.Column(c).Grow(1).MinWidth(0).Gap(11).Padding(14).Radius(14).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, icon).Size(16, 16).TextColor(theme.Accent)
			ui.Text(c, title).FontSize(13).Bold()
		})
		value := path
		if value == "" {
			value = "No folder selected"
		}
		ui.Text(c, value).FontSize(12).SingleLine().Tooltip(value).TextColor(theme.TextMuted).FillWidth()
		choose := ui.Button(c.Key("folder-choose-"+title), "").FillWidth().Padding(9, 10).Gap(8).Radius(9).
			Label(button)
		choose.Children(func() {
			ui.Icon(c, iconFolder).Size(15, 15)
			ui.Text(c, button).FontSize(12)
		})
		if choose.Clicked() {
			action()
		}
	})
}

func containsSpace(db task.Database, id string) bool {
	for _, space := range db.Spaces {
		if space.ID == id {
			return true
		}
	}
	return false
}

func findDot(db task.Database, id string) (task.Dot, bool) {
	for _, dot := range db.Dots {
		if dot.ID == id {
			return dot, true
		}
	}
	return task.Dot{}, false
}

func selectedSpaceName(db task.Database, id string) string {
	for _, space := range db.Spaces {
		if space.ID == id {
			return space.Name
		}
	}
	return "None"
}

func statusGlyph(status task.Status) string {
	switch status {
	case task.Done:
		return "✓"
	case task.Running:
		return "●"
	case task.Failed:
		return "!"
	case task.Paused:
		return "Ⅱ"
	case task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return "◷"
	default:
		return "○"
	}
}

func humanStatus(status task.Status) string {
	return strings.ReplaceAll(string(status), "_", " ")
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return "…" + value[len(value)-max+1:]
}

func errorsIsCanceled(err error) bool {
	return err == context.Canceled
}

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := x-16, y-16
			if dx*dx+dy*dy <= 12*12 {
				img.Set(x, y, color.Black)
			}
		}
	}
	var b strings.Builder
	_ = png.Encode(writer{&b}, img)
	return []byte(b.String())
}

type writer struct{ b *strings.Builder }

func (w writer) Write(p []byte) (int, error) { return w.b.Write(p) }
