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

type app struct {
	db  *store.Store
	win *mygo.Window

	snapshot          task.Database
	selectedSpaceID   string
	selectedDotID     string
	selectedRoutineID string

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

func (a *app) view(c *ui.Context) {
	theme := c.Theme()
	ui.Row(c).Fill().Children(func() {
		a.sidebar(c)
		ui.Divider(c)
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(18).Padding(28).Children(func() {
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
}

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(236).FillHeight().Gap(12).Padding(18).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "AETHER").FontSize(17).Bold()
			ui.Badge(c, "DOTS")
		})
		ui.Text(c, "SPACES").FontSize(11).Bold()
		for _, space := range a.snapshot.Spaces {
			label := space.Name
			if space.ID == a.selectedSpaceID {
				label = "●  " + label
			}
			if ui.Button(c, label).FillWidth().Clicked() {
				a.selectedSpaceID = space.ID
				a.selectedDotID = ""
				a.selectedRoutineID = ""
				a.refresh()
			}
		}
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &a.spaceDraft).Placeholder("Space name").Width(124)
			if ui.Button(c, "Add").Clicked() {
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
		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "RECENT DOTS").FontSize(11).Bold()
			ui.Spacer(c)
			if ui.Button(c, "+ New").Clicked() {
				a.beginCapture(false)
			}
		})
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				for i := len(a.snapshot.Dots) - 1; i >= 0; i-- {
					dot := a.snapshot.Dots[i]
					label := statusGlyph(dot.Status) + "  " + dot.Name
					if ui.Button(c, label).FillWidth().Clicked() {
						a.selectedDotID = dot.ID
						a.selectedRoutineID = ""
						a.selectedSpaceID = dot.SpaceID
						a.refresh()
					}
				}
			})
		})
		if len(a.snapshot.Routines) > 0 {
			ui.Divider(c)
			ui.Text(c, "ROUTINES").FontSize(11).Bold()
			for _, routine := range a.snapshot.Routines {
				if ui.Button(c, "↻  "+routine.Name).FillWidth().Clicked() {
					a.selectedRoutineID = routine.ID
					a.selectedDotID = ""
					a.selectedSpaceID = ""
					a.routineName = routine.Name
					a.routineObjective = routine.Objective
					a.refresh()
				}
			}
		}
		ui.Spacer(c)
		ui.Text(c, "Local runtime · this device").FontSize(11)
	})
}

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "AETHER Dots").FontSize(24).Bold()
			ui.Text(c, "One outcome. One permission scope. One resumable history.").FontSize(13).TextColor(theme.TextMuted)
		})
		ui.Spacer(c)
		ui.Badge(c, "⌘ ⇧ SPACE  CAPTURE")
	})
	if a.message != "" {
		ui.Text(c, a.message).TextColor(theme.TextMuted).FillWidth()
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
			folderCard(c, "Source folder", a.sourceRoot, "Choose source…", func() { a.pickFolder(true) })
			folderCard(c, "Output folder", a.outputDir, "Choose destination…", func() { a.pickFolder(false) })
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, "Create Dot").Disabled(strings.TrimSpace(a.objective) == "" || a.sourceRoot == "" || a.outputDir == "").Clicked() {
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
	ui.Fieldset(c, "OpenAI-compatible model", func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, fmt.Sprintf("Endpoint: %s  ·  API: %s  ·  model: %s", cfg.EndpointLabel(), cfg.API, cfg.Model)).FillWidth()
			keyState := "missing"
			if cfg.APIKey != "" {
				keyState = "available in process environment"
			}
			ui.Text(c, fmt.Sprintf("Credential: %s (%s)", cfg.KeyEnv, keyState)).TextColor(theme.TextMuted)
			ui.Text(c, "Configure AETHER_MODEL_BASE_URL, AETHER_MODEL_NAME, AETHER_MODEL_API, AETHER_MODEL_API_KEY_ENV, and the named key variable before model runs.").FontSize(12).TextColor(theme.TextMuted).FillWidth()
			if ui.Button(c, "Test model connection").Clicked() {
				a.testModel()
			}
		})
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

func folderCard(c *ui.Context, title, path, button string, action func()) {
	ui.Fieldset(c, title, func() {
		ui.Column(c).Gap(8).Children(func() {
			value := path
			if value == "" {
				value = "No folder selected"
			}
			ui.Text(c, value).FontSize(12).FillWidth()
			if ui.Button(c, button).Clicked() {
				action()
			}
		})
	}).Grow(1)
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
