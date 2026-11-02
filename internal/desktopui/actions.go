package desktopui

import (
	"context"

	"aether-dots/internal/engine"
)

func (a *app) startRun(id string) {
	if a.cancelRun != nil {
		return
	}
	dot, ok := a.db.GetDot(id)
	if !ok {
		a.adt.Error("Dot message not found", "This item may have been removed. Refresh the workspace and try again.")
		return
	}
	conversationID := dot.ConversationID
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.runningDotID = id
	a.live = ""
	toastID := "dot-run-" + id
	a.adt.Loading(toastID, "Your Dot is working", "A reply is being prepared.")
	if conversationID == a.selectedConversationID {
		a.conversationError = ""
	}
	win := a.win
	prefs := a.prefs
	go func() {
		generator := generatorForPreferences(prefs)
		err := engine.RunWithProgress(ctx, a.db, id, generator, func(delta string) {
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
		cancel()
		finish := func() {
			a.cancelRun = nil
			a.runningDotID = ""
			a.live = ""
			a.refresh()
			if err == nil {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply saved", Description: "Your Dot’s response is in the conversation.", Level: ADTSuccess})
			} else if errorsIsCanceled(err) {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply paused", Description: "You can ask your Dot to try again when you’re ready.", Level: ADTInfo})
			} else {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply failed", Description: err.Error(), Level: ADTError})
			}
			if conversationID != "" && conversationID == a.selectedConversationID {
				if errorsIsCanceled(err) {
					a.conversationError = ""
				} else if err != nil {
					a.conversationError = err.Error()
				} else {
					a.conversationError = ""
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
