package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/integrations/computeruse"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

// TestLiveComputerUseMusicControl is opt-in because it controls the user's
// current Apple Music playback. It uses a temporary store and restores the
// playback state observed at the beginning of the test.
func TestLiveComputerUseMusicControl(t *testing.T) {
	if os.Getenv("AETHER_RUN_CUA_LIVE") != "1" {
		t.Skip("set AETHER_RUN_CUA_LIVE=1 to exercise the installed Cua Driver")
	}
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "CUA live diagnostic"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Exercise bounded Music playback controls")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Running, "Started live Computer Use diagnostic."); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateConversationProfileAndIntegrations(conversation.ID, spaceID,
		conversation.Name, conversation.Icon, conversation.Color, conversation.Purpose, conversation.Instructions,
		nil, nil, false, []string{task.ComputerUseScopeMusicControl}, true); err != nil {
		t.Fatal(err)
	}

	service := computeruse.New(filepath.Join(dir, "computer-use"))
	defer service.Stop()
	tool := musicControlTool(db, dot.ID, service, "aether-live-diagnostic", service.Generation())
	call := func(action string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return tool.Handler(ctx, `{"action":"`+action+`"}`)
	}

	initialPause, err := call("pause")
	if err != nil {
		t.Fatalf("AETHER Music pause tool failed: %v", err)
	}
	wasPlaying := !strings.Contains(strings.ToLower(initialPause), "already paused")
	defer func() {
		if !wasPlaying {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := tool.Handler(ctx, `{"action":"play"}`); err != nil {
			t.Errorf("could not restore Music's initial playing state: %v", err)
		}
	}()

	playResult, err := call("play")
	if err != nil {
		t.Fatalf("AETHER Music play tool failed: %v", err)
	}
	if strings.Contains(strings.ToLower(playResult), "already playing") {
		t.Fatalf("Music was not paused before the play action: %q", playResult)
	}
	pauseResult, err := call("pause")
	if err != nil {
		t.Fatalf("AETHER Music pause restoration failed: %v", err)
	}
	if strings.Contains(strings.ToLower(pauseResult), "already paused") {
		t.Fatalf("Music did not enter the playing state before pause restoration: %q", pauseResult)
	}

	snapshot := db.Snapshot()
	for _, storedDot := range snapshot.Dots {
		if storedDot.ID == dot.ID {
			if len(storedDot.Events) < 3 {
				t.Fatalf("AETHER recorded %d Music access events, want at least 3", len(storedDot.Events))
			}
			t.Log("AETHER per-Dot Music tool exercised bounded pause/play/pause controls; cleanup restores the initial playback state when needed")
			return
		}
	}
	t.Fatal("diagnostic Dot was not persisted")
}
