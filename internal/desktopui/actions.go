package desktopui

import (
	"context"

	"aether-dots/internal/engine"
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

func (a *app) startRun(id string) {
	if a.cancelRun != nil {
		return
	}
	dot, ok := a.db.GetDot(id)
	if !ok {
		a.message = "Dot not found."
		return
	}
	conversationID := dot.ConversationID
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.runningDotID = id
	a.live = ""
	a.message = "Run started; events are saved as work progresses."
	if conversationID != "" && conversationID == a.selectedConversationID {
		a.conversationError = ""
	}
	win := a.win
	modelName := a.prefs.Model
	go func() {
		generator := openAIGenerator(modelName)
		err := engine.RunWithProgress(ctx, a.db, id, &generator, func(delta string) {
			if win != nil {
				win.Update(func() {
					if a.runningDotID == id {
						a.live = tail(a.live+delta, 6000)
					}
				})
			}
		}, func() {
			if win != nil {
				win.Update(func() {
					if a.runningDotID == id {
						a.refresh()
					}
				})
			}
		})
		resultErr := a.persistConversationResult(conversationID, id, err)
		cancel()
		finish := func() {
			a.cancelRun = nil
			a.runningDotID = ""
			a.live = ""
			a.refresh()
			if err == nil && resultErr == nil {
				a.message = "Run finished; the local artifact was byte-verified."
			} else if resultErr != nil {
				a.message = "Run completed, but its verified artifact could not be added to the conversation: " + resultErr.Error()
			} else if errorsIsCanceled(err) {
				a.message = "Run paused at a safe boundary. Resume to re-read the selected local source."
			} else if err != nil {
				a.message = err.Error()
			}
			if win != nil && conversationID != "" && conversationID == a.selectedConversationID {
				switch {
				case err == nil && resultErr == nil:
					a.conversationError = ""
				case resultErr != nil:
					a.conversationError = resultErr.Error()
				case errorsIsCanceled(err):
					a.conversationError = ""
				case err != nil:
					a.conversationError = err.Error()
				}
				if err == nil && resultErr == nil {
					a.followChatIfAtBottom()
				}
			}
		}
		if win == nil {
			finish()
			return
		}
		win.Update(finish)
	}()
}
