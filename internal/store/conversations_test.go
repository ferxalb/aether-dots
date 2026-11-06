package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aether-dots/internal/task"
)

func TestConversationProfileAndRepliesPersistWithoutFolderGrants(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if conversation.Name != "New Dot" || conversation.Icon == "" || conversation.Color == "" {
		t.Fatalf("new Dot defaults are incomplete: %#v", conversation)
	}
	if err := db.UpdateConversationProfile(conversation.ID, conversation.SpaceID, "Decision partner", "◈", "#1D69C8", "Prepare concise decision briefs", "Use plain language and cite uncertainty."); err != nil {
		t.Fatal(err)
	}
	conversation, ok := findConversationForTest(db.Snapshot(), conversation.ID)
	if !ok || conversation.Name != "Decision partner" || conversation.Icon != "◈" || conversation.Color != "#1D69C8" {
		t.Fatalf("profile update was not saved: %#v", conversation)
	}

	first, firstMessage, err := db.CreateConversationRun(conversation.ID, "Summarize the open decisions")
	if err != nil {
		t.Fatal(err)
	}
	if firstMessage.Role != "user" || firstMessage.DotID != first.ID || first.ParentDotID != "" {
		t.Fatalf("first message/run link is incomplete: message=%#v dot=%#v", firstMessage, first)
	}
	for _, part := range []string{"Dot name:\nDecision partner", "Dot objective:\nPrepare concise decision briefs", "Dot working instructions:\nUse plain language", "Newest user message:\nSummarize the open decisions"} {
		if !strings.Contains(first.Objective, part) {
			t.Errorf("message objective missing %q: %q", part, first.Objective)
		}
	}
	if strings.Contains(first.Objective, "Recent conversation context") {
		t.Fatal("first message included nonexistent prior context")
	}

	if err := db.SetStatus(first.ID, task.Running, "Reply started."); err != nil {
		t.Fatal(err)
	}
	reply := "There are two open decisions."
	if err := db.AppendConversationResult(conversation.ID, first.ID, reply); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendConversationResult(conversation.ID, first.ID, reply); err != nil {
		t.Fatalf("saving the same reply should be idempotent: %v", err)
	}
	if err := db.SetStatus(first.ID, task.Done, "Reply saved."); err != nil {
		t.Fatal(err)
	}

	second, secondMessage, err := db.CreateConversationRun(conversation.ID, "Make that shorter")
	if err != nil {
		t.Fatal(err)
	}
	if second.ParentDotID != first.ID || secondMessage.DotID != second.ID {
		t.Fatalf("follow-up was not linked to its previous run: second=%#v message=%#v", second, secondMessage)
	}
	if !second.ConversationContextDeferred {
		t.Fatal("follow-up prompt should assemble prior context at request time")
	}
	prompt, err := db.ConversationObjective(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"Recent saved conversation context", "Assistant reply:\n" + reply, "Newest user message:\nMake that shorter"} {
		if !strings.Contains(prompt, part) {
			t.Errorf("follow-up prompt missing %q: %q", part, prompt)
		}
	}

	snapshot := db.Snapshot()
	if len(snapshot.Conversations) != 1 || len(snapshot.Conversations[0].Messages) != 3 || len(snapshot.Dots) != 2 {
		t.Fatalf("persisted conversation structure is wrong: conversations=%d messages=%d dots=%d", len(snapshot.Conversations), len(snapshot.Conversations[0].Messages), len(snapshot.Dots))
	}
	reopened, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	reloaded, ok := findConversationForTest(reopened.Snapshot(), conversation.ID)
	if !ok || len(reloaded.Messages) != 3 || reloaded.Messages[1].Content != reply {
		t.Fatalf("conversation messages did not survive reopen: %#v", reloaded)
	}
	data, err := json.Marshal(reopened.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"source_root", "output_dir", "artifacts", "routines"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("new store still persists retired field %q: %s", forbidden, data)
		}
	}
}

func TestConversationContextIsBoundedAndKeepsNewestMessage(t *testing.T) {
	var messages []task.ConversationMessage
	for i := 0; i < 15; i++ {
		messages = append(messages, task.ConversationMessage{Role: "user", Content: strings.Repeat("message ", 800)})
	}
	conversation := task.Conversation{Name: "Bounded", Purpose: "Keep the saved objective", Messages: messages}
	objective := conversationObjective(conversation, "Newest request stays intact")
	if len(objective) > task.MaxConversationObjectiveBytes {
		t.Fatalf("conversation objective exceeded its byte cap: %d", len(objective))
	}
	if !strings.Contains(objective, "Recent conversation context") || !strings.Contains(objective, "Newest request stays intact") || !strings.Contains(objective, conversation.Purpose) {
		t.Fatalf("bounded objective omitted required inputs: %q", objective)
	}
	if strings.Count(objective, "User request:") > task.MaxConversationContextMessages {
		t.Fatalf("objective included too many prior messages: %d", strings.Count(objective, "User request:"))
	}
	if !strings.Contains(objective, "[excerpt omitted]") {
		t.Fatal("long prior messages were not marked as excerpts")
	}
	conversation.Purpose = strings.Repeat("p", task.MaxConversationPurposeBytes)
	conversation.Instructions = strings.Repeat("i", task.MaxConversationInstructionsBytes)
	conversation.Messages = []task.ConversationMessage{{Role: "assistant", Content: strings.Repeat("a", 2<<20)}}
	newest := strings.Repeat("n", task.MaxConversationMessageBytes)
	maxObjective := conversationObjective(conversation, newest)
	if len(maxObjective) > task.MaxConversationObjectiveBytes || !strings.Contains(maxObjective, conversation.Purpose) || !strings.HasSuffix(maxObjective, newest) {
		t.Fatalf("objective truncated the saved profile/newest message or exceeded its cap: bytes=%d", len(maxObjective))
	}
}

func TestConversationSendRejectsParallelActiveRun(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := db.CreateConversationRun(conversation.ID, "Current request")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(active.ID, task.Running, "Started."); err != nil {
		t.Fatal(err)
	}
	const attempts = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	rejected := 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := db.CreateConversationRun(conversation.ID, "Concurrent request "+string(rune('a'+i)))
			if err != nil {
				mu.Lock()
				rejected++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if rejected != attempts {
		t.Fatalf("got %d rejected sends, want %d", rejected, attempts)
	}
	if got := db.Snapshot(); len(got.Dots) != 1 || len(got.Conversations[0].Messages) != 1 {
		t.Fatalf("active run accumulated duplicate chat sends: %#v", got)
	}
}

func TestConcurrentConversationSubmissionsReserveOneRunAtomically(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes, failures := 0, 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := db.CreateConversationRun(conversation.ID, "Concurrent request "+string(rune('a'+i)))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				failures++
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 || failures != attempts-1 {
		t.Fatalf("concurrent sends created %d runs and rejected %d; want one run and %d rejections", successes, failures, attempts-1)
	}
}

func TestRecoverInterruptedConversationRuns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Run this request")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetCheckpoint(dot.ID, "partial response retained for review"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Running, "Started."); err != nil {
		t.Fatal(err)
	}
	simulateCrash(db, dot.ID)
	recovered, err := db.RecoverInterruptedRuns()
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v, want one active conversation run", recovered, err)
	}
	got, _ := db.GetDot(dot.ID)
	if got.Status != task.Paused || got.Checkpoint != "partial response retained for review" {
		t.Fatalf("recovery state=%#v", got)
	}
	if len(got.Events) < 2 || got.Events[len(got.Events)-1].Type != "interrupted" {
		t.Fatalf("interruption event missing: %#v", got.Events)
	}
}

func TestCreateConversationProfileValidatesTextLimits(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateConversationProfile(conversation.ID, conversation.SpaceID, strings.Repeat("x", task.MaxConversationNameBytes+1), "✦", "#7458D8", "", ""); err == nil {
		t.Fatal("overlong Dot name was accepted")
	}
	if err := db.UpdateConversationProfile(conversation.ID, conversation.SpaceID, "Valid", "✦", "#7458D8", "", strings.Repeat("x", task.MaxConversationInstructionsBytes+1)); err == nil {
		t.Fatal("overlong instructions were accepted")
	}
}

func TestRecoverRunDoesNotDropConversationTimestamp(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Time check"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "A message")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	if err := db.SetStatus(dot.ID, task.Running, "Started."); err != nil {
		t.Fatal(err)
	}
	simulateCrash(db, dot.ID)
	if _, err := db.RecoverInterruptedRuns(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := db.GetDot(dot.ID)
	if reloaded.UpdatedAt.Before(started) {
		t.Fatalf("updated_at moved backwards: %s < %s", reloaded.UpdatedAt, started)
	}
}

func findConversationForTest(db task.Database, id string) (task.Conversation, bool) {
	for _, conversation := range db.Conversations {
		if conversation.ID == id {
			return conversation, true
		}
	}
	return task.Conversation{}, false
}
