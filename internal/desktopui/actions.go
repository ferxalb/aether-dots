package desktopui

import (
	"context"
	"strings"
	"time"

	"aether-dots/internal/engine"
	"github.com/egoist/mygo"
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
		onProgress := func() {
			if win != nil {
				win.Update(func() {
					if a.runningDotID == id {
						a.refresh()
					}
				})
			}
		}
		generator := generatorForConversationRun(prefs, a.db, id, a.githubService, onProgress)
		var deltas chan string
		var deltasDone chan struct{}
		if win != nil {
			deltas = make(chan string, 128)
			deltasDone = make(chan struct{})
			go collectRunDeltas(win, a, id, deltas, deltasDone)
		}
		err := engine.RunWithProgress(ctx, a.db, id, generator, func(delta string) {
			if deltas != nil {
				deltas <- delta
			}
		}, onProgress)
		if deltas != nil {
			close(deltas)
			<-deltasDone
		}
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

func collectRunDeltas(win *mygo.Window, a *app, id string, deltas <-chan string, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	var streamed strings.Builder
	lastPublished := 0
	publish := func() {
		if streamed.Len() == lastPublished {
			return
		}
		snapshot := streamed.String()
		lastPublished = len(snapshot)
		win.Update(func() {
			if a.runningDotID == id {
				a.live = snapshot
			}
		})
	}
	for {
		select {
		case delta, ok := <-deltas:
			if !ok {
				publish()
				return
			}
			streamed.WriteString(delta)
		case <-ticker.C:
			publish()
		}
	}
}
