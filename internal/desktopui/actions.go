package desktopui

import (
	"context"

	"aether-dots/internal/engine"
	"aether-dots/internal/task"
	"github.com/egoist/mygo"
)

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
	a.settingsOpen = false
	a.message = "Dot captured and durably saved."
	a.refresh()
}

func (a *app) beginCapture(preserveScope bool) {
	a.selectedDotID, a.selectedRoutineID = "", ""
	a.settingsOpen = false
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
	modelName := a.prefs.Model
	go func() {
		generator := openAIGenerator(modelName)
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
