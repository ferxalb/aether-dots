package desktopui

import (
	"fmt"
	"strings"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) dotView(c *ui.Context, theme *ui.Theme, dot task.Dot) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, dot.Name).FontSize(23).Bold().FillWidth()
			ui.Badge(c, humanStatus(dot.Status))
		})
		ui.Text(c, "Space: "+selectedSpaceName(a.snapshot, dot.SpaceID)).FontSize(12).TextColor(theme.TextMuted)
		ui.Text(c, dot.Objective).FontSize(15).FillWidth()

		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
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
			taskPanel(c, theme, "Live model draft", func() {
				ui.Text(c, tail(a.live, 1800)).FontSize(13).FillWidth()
			})
		}

		if len(dot.Artifacts) > 0 {
			taskPanel(c, theme, "Result", func() {
				if dot.Mode == "model" {
					ui.Text(c, "Model draft · review its claims against the source documents.").FontSize(12).TextColor(theme.TextMuted).FillWidth()
				}
				for _, artifact := range dot.Artifacts {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, artifact.Path).FontSize(12).FillWidth().Tooltip(artifact.Path)
						ui.Text(c, fmt.Sprintf("Verified · SHA-256 %s · %d bytes", artifact.SHA256, artifact.Bytes)).FontSize(11).TextColor(theme.TextMuted).FillWidth()
					})
				}
			})
		}

		if dot.Status == task.Done && dot.RoutineID == "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
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
				ui.TextInput(c, &a.routineName).Placeholder("Routine name").Width(220).Label("Routine name")
			})
		}

		taskScopePanel(c, theme, dot.SourceRoot, dot.OutputDir, dot.Mode, a.prefs.Model)

		taskPanel(c, theme, "Progress", func() {
			if dot.Checkpoint != "" {
				ui.Text(c, "Current step · "+tail(dot.Checkpoint, 160)).FontSize(12).TextColor(theme.TextMuted).FillWidth()
			}
			if len(dot.Events) == 0 {
				ui.Text(c, "No progress recorded yet.").FontSize(12).TextColor(theme.TextMuted)
				return
			}
			ui.Text(c, fmt.Sprintf("%d events", len(dot.Events))).FontSize(11).TextColor(theme.TextMuted)
			ui.Scroll(c).FillWidth().Height(200).Children(func() {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					for _, event := range dot.Events {
						ui.Row(c).AlignItems(ui.Start).Gap(10).Children(func() {
							ui.Text(c, fmt.Sprintf("%02d", event.Sequence)).FontSize(11).TextColor(theme.TextMuted).Width(24)
							ui.Column(c).Gap(2).FillWidth().Children(func() {
								ui.Text(c, strings.ReplaceAll(event.Type, "_", " ")).FontSize(11).Bold().TextColor(theme.TextMuted)
								ui.Text(c, event.Message).FontSize(12).FillWidth()
							})
						})
					}
				})
			})
		})
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
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, selected.Name).FontSize(22).Bold()
			ui.Text(c, fmt.Sprintf("Routine · version %d · promoted from Dot %s", selected.Version, selected.SourceDotID)).FontSize(12).TextColor(theme.TextMuted)
		})
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			ui.Text(c, "Name").FontSize(12).Bold().TextColor(theme.TextMuted)
			ui.TextInput(c.Key("routine-name"), &a.routineName).FillWidth().Label("Routine name")
			ui.Text(c, "Outcome").FontSize(12).Bold().TextColor(theme.TextMuted)
			ui.TextArea(c.Key("routine-objective"), &a.routineObjective).MinHeight(92).FillWidth().Label("Routine outcome")
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			if ui.PrimaryButton(c, "Save Routine").Clicked() {
				if err := a.db.EditRoutine(selected.ID, a.routineName, a.routineObjective); err != nil {
					a.message = err.Error()
				} else {
					a.message = "Routine updated. Its grants remain unchanged."
				}
				a.refresh()
			}
			if ui.Button(c, "Run Routine").Clicked() {
				dot, err := a.db.DotFromRoutine(selected.ID, a.selectedSpaceID)
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
		taskPanel(c, theme, "Fixed local permissions", func() {
			taskScopeLine(c, theme, "Read only", selected.SourceRoot)
			taskScopeLine(c, theme, "Write only", selected.OutputDir)
			ui.Divider(c)
			ui.Text(c, "Precondition").FontSize(11).Bold().TextColor(theme.TextMuted)
			ui.Text(c, selected.Precondition).FontSize(12).FillWidth()
			ui.Text(c, "Verification").FontSize(11).Bold().TextColor(theme.TextMuted)
			ui.Text(c, selected.Verification).FontSize(12).FillWidth()
		})
	})
}
