package desktopui

import (
	"context"
	"strings"
	"time"

	"aether-dots/internal/engine"
	"github.com/egoist/mygo"
)

func (a *app) initializeRunState() {
	if a.runCancels == nil {
		a.runCancels = make(map[string]context.CancelFunc)
	}
	if a.liveReplies == nil {
		a.liveReplies = make(map[string]string)
	}
}

func (a *app) hasActiveRuns() bool {
	return len(a.runCancels) > 0
}

func (a *app) activeRunDotIDForConversation(conversationID string) string {
	for id := range a.runCancels {
		if strings.HasPrefix(id, "routine:") {
			continue
		}
		if dot, ok := a.db.GetDot(id); ok && dot.ConversationID == conversationID {
			return id
		}
	}
	return ""
}

func (a *app) cancelRunByID(id string) bool {
	cancel := a.runCancels[id]
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (a *app) startRun(id string) {
	a.initializeRunState()
	if a.runCancels[id] != nil {
		return
	}
	dot, ok := a.db.GetDot(id)
	if !ok {
		a.adt.Error("Dot message not found", "This item may have been removed. Refresh the workspace and try again.")
		return
	}
	conversationID := dot.ConversationID
	if conversationID != "" && a.activeRunDotIDForConversation(conversationID) != "" {
		a.adt.Warning("This Dot is already replying", "Wait for this conversation to finish before sending another message.")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.runCancels[id] = cancel
	a.liveReplies[id] = ""
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
				win.Update(func() { a.refresh() })
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
			delete(a.runCancels, id)
			delete(a.liveReplies, id)
			a.refresh()
			if err == nil {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply saved", Description: "Your Dot’s response is in the conversation.", Level: ADTSuccess})
			} else if errorsIsCanceled(err) {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply paused", Description: "You can ask your Dot to try again when you’re ready.", Level: ADTInfo})
			} else {
				a.adt.Push(ADTNotice{ID: toastID, Title: "Reply failed", Description: err.Error(), Level: ADTError})
			}
			if conversationID != "" && conversationID == a.selectedConversationID {
				if errorsIsCanceled(err) || err == nil {
					a.conversationError = ""
				} else {
					a.conversationError = err.Error()
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
			if a.runCancels[id] != nil {
				a.liveReplies[id] = snapshot
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
