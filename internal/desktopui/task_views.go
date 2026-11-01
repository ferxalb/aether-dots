package desktopui

import (
	"fmt"
	"strings"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

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
				ui.Text(c, "Selected source text is sent to OpenAI using the model configured in Settings. The API key is read from the system credential store or OPENAI_API_KEY.").TextColor(theme.TextMuted)
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
			if dot.Mode == "model" && ui.Button(c, "OpenAI settings").Clicked() {
				a.openSettings()
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
					ui.Text(c, "Model · selected text is sent to OpenAI (Responses API, "+a.prefs.Model+")").TextColor(theme.TextMuted)
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
