package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aether-dots/internal/task"
)

func TestConversationFollowupsKeepGrantsAndPersistVerifiedResults(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := conversationFolders(t, dir)
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}

	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Decision partner",
		Purpose: "Help me prepare concise decision briefs", Mode: "model",
		SourceRoot: sourceDir, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(conversation.Messages) != 0 {
		t.Fatalf("new Dot should open with an empty chat, got %d messages", len(conversation.Messages))
	}

	first, firstMessage, err := db.CreateConversationRun(conversation.ID, "Summarize the three open decisions")
	if err != nil {
		t.Fatal(err)
	}
	if firstMessage.Role != "user" || firstMessage.DotID != first.ID || first.ParentDotID != "" {
		t.Fatalf("first message/run link is incomplete: message=%#v dot=%#v", firstMessage, first)
	}
	if !strings.Contains(first.Objective, conversation.Purpose) || !strings.Contains(first.Objective, firstMessage.Content) {
		t.Fatalf("bounded objective is missing the saved purpose or newest request: %q", first.Objective)
	}
	if strings.Contains(first.Objective, "Recent conversation context") {
		t.Fatal("first model run included a nonexistent prior transcript")
	}

	result := "# Decision brief\n\nThe team needs to choose a default."
	sum := sha256.Sum256([]byte(result))
	artifact := task.Artifact{
		Path:   filepath.Join(outputDir, "decision-brief-"+first.ID+".md"),
		SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(result)), VerifiedAt: time.Now().UTC(),
	}
	if err := db.AddArtifact(first.ID, artifact); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(first.ID, task.Done, "Verified test artifact."); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendConversationResult(conversation.ID, first.ID, result); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendConversationResult(conversation.ID, first.ID, result); err != nil {
		t.Fatalf("saving the same completed result should be idempotent: %v", err)
	}
	if err := db.AppendConversationResult(conversation.ID, first.ID, result+" altered"); err == nil {
		t.Fatal("an assistant result that differs from the verified artifact was accepted")
	}

	second, secondMessage, err := db.CreateConversationRun(conversation.ID, "Make it shorter")
	if err != nil {
		t.Fatal(err)
	}
	if second.ParentDotID != first.ID || secondMessage.DotID != second.ID {
		t.Fatalf("follow-up was not linked to its previous real run: second=%#v message=%#v", second, secondMessage)
	}
	if second.SpaceID != first.SpaceID || second.SourceRoot != sourceDir || second.OutputDir != outputDir || second.Mode != "model" {
		t.Fatalf("follow-up changed the conversation's grants or mode: %#v", second)
	}
	for _, part := range []string{
		"Dot purpose:\n" + conversation.Purpose,
		"Recent conversation context",
		"User request:\nSummarize the three open decisions",
		"Assistant result from a verified artifact:\n" + result,
		"Newest delegation:\nMake it shorter",
	} {
		if !strings.Contains(second.Objective, part) {
			t.Errorf("second model objective does not contain prior context %q:\n%s", part, second.Objective)
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
	reloaded := reopened.Snapshot()
	if len(reloaded.Conversations) != 1 || len(reloaded.Conversations[0].Messages) != 3 || reloaded.Conversations[0].Messages[1].Content != result {
		t.Fatalf("conversation messages did not survive reopen: %#v", reloaded.Conversations)
	}
}

func TestConversationContextIsBoundedAndLocalModeDoesNotClaimChatReasoning(t *testing.T) {
	var messages []task.ConversationMessage
	for i := 0; i < 12; i++ {
		messages = append(messages, task.ConversationMessage{Role: "user", Content: strings.Repeat("message ", 800)})
	}
	conversation := task.Conversation{Purpose: "Keep the saved purpose", Mode: "model", Messages: messages}
	objective := conversationObjective(conversation, "Newest request stays intact")
	if len(objective) > task.MaxConversationObjectiveBytes {
		t.Fatalf("conversation objective exceeded its byte cap: %d", len(objective))
	}
	if !strings.Contains(objective, "older messages may be omitted") || !strings.Contains(objective, "Newest request stays intact") || !strings.Contains(objective, conversation.Purpose) {
		t.Fatalf("bounded objective omitted required disclosure or newest inputs: %q", objective)
	}
	if strings.Count(objective, "User request:") > task.MaxConversationContextMessages {
		t.Fatalf("objective included too many prior messages: %d", strings.Count(objective, "User request:"))
	}
	if !strings.Contains(objective, "[excerpt omitted]") {
		t.Fatal("long prior messages were not marked as excerpts")
	}
	conversation.Purpose = strings.Repeat("p", task.MaxConversationPurposeBytes)
	conversation.Messages = []task.ConversationMessage{{Role: "assistant", Content: strings.Repeat("a", 2<<20)}}
	maxPurposeObjective := conversationObjective(conversation, strings.Repeat("n", task.MaxConversationMessageBytes))
	if len(maxPurposeObjective) > task.MaxConversationObjectiveBytes ||
		!strings.Contains(maxPurposeObjective, conversation.Purpose) ||
		!strings.HasSuffix(maxPurposeObjective, strings.Repeat("n", task.MaxConversationMessageBytes)) {
		t.Fatalf("objective truncated the saved purpose/newest request or exceeded its cap: bytes=%d", len(maxPurposeObjective))
	}

	conversation.Mode = "local"
	localObjective := conversationObjective(conversation, "Newest local request")
	if strings.Contains(localObjective, "Recent conversation context") || !strings.Contains(localObjective, "Newest local request") {
		t.Fatalf("local extraction should not claim prior-chat reasoning: %q", localObjective)
	}
}

func TestConversationSendRejectsParallelActiveRun(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := conversationFolders(t, dir)
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Partner", Purpose: "Prepare briefs",
		Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir,
	})
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
	errors := 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := db.CreateConversationRun(conversation.ID, "Concurrent request "+string(rune('a'+i)))
			if err != nil {
				mu.Lock()
				errors++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if errors != attempts {
		t.Fatalf("got %d rejected sends, want %d", errors, attempts)
	}
	snapshot := db.Snapshot()
	if len(snapshot.Dots) != 1 || len(snapshot.Conversations[0].Messages) != 1 {
		t.Fatalf("active run accumulated duplicate chat sends: dots=%d messages=%d", len(snapshot.Dots), len(snapshot.Conversations[0].Messages))
	}
}

func TestConcurrentConversationSubmissionsReserveOneRunAtomically(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := conversationFolders(t, dir)
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Partner", Purpose: "Prepare briefs",
		Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir,
	})
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
	snapshot := db.Snapshot()
	if len(snapshot.Dots) != 1 || len(snapshot.Conversations[0].Messages) != 1 || snapshot.Dots[0].Status != task.Ready {
		t.Fatalf("atomic run reservation was not persisted: dots=%#v messages=%#v", snapshot.Dots, snapshot.Conversations[0].Messages)
	}
}

func TestCreateConversationRequiresAvailableDisjointFolders(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := conversationFolders(t, dir)
	insideOutput := filepath.Join(sourceDir, "generated")
	if err := os.MkdirAll(insideOutput, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	base := task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Folder scoped Dot", Purpose: "Read and write safely",
		Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir,
	}
	if _, err := db.CreateConversation(base); err != nil {
		t.Fatalf("valid selected folders were rejected: %v", err)
	}
	base.OutputDir = insideOutput
	if _, err := db.CreateConversation(base); err == nil {
		t.Fatal("output folder inside the source folder was accepted")
	}
	base.OutputDir = filepath.Join(dir, "missing")
	if _, err := db.CreateConversation(base); err == nil {
		t.Fatal("nonexistent output folder was accepted")
	}
	if len(db.Snapshot().Conversations) != 1 {
		t.Fatalf("invalid setup created a conversation: %#v", db.Snapshot().Conversations)
	}
}

func TestRecoverInterruptedConversationRunsOnly(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := conversationFolders(t, dir)
	db, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: "Partner", Purpose: "Prepare briefs", Mode: "local",
		SourceRoot: sourceDir, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversationDot, _, err := db.CreateConversationRun(conversation.ID, "Run this request")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetCheckpoint(conversationDot.ID, "partial model draft retained for review"); err != nil {
		t.Fatal(err)
	}
	legacyDot, err := db.CreateDot(task.CreateDot{
		SpaceID: spaceID, Objective: "Legacy task", Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dot := range []task.Dot{conversationDot, legacyDot} {
		if err := db.SetStatus(dot.ID, task.Running, "Started."); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := db.RecoverInterruptedRuns()
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v, want one linked conversation run", recovered, err)
	}
	gotConversation, _ := db.GetDot(conversationDot.ID)
	gotLegacy, _ := db.GetDot(legacyDot.ID)
	if gotConversation.Status != task.Paused || gotLegacy.Status != task.Running {
		t.Fatalf("recovery changed unexpected states: conversation=%s legacy=%s", gotConversation.Status, gotLegacy.Status)
	}
	if gotConversation.Checkpoint != "partial model draft retained for review" {
		t.Fatalf("recovery discarded the saved run checkpoint: %q", gotConversation.Checkpoint)
	}
	if len(gotConversation.Events) < 2 || gotConversation.Events[len(gotConversation.Events)-1].Type != "interrupted" {
		t.Fatalf("interruption event missing: %#v", gotConversation.Events)
	}
}

func TestLegacyDotsJSONWithoutConversationsRemainsReadable(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	dot, err := s.CreateDot(task.CreateDot{
		SpaceID: s.Snapshot().Spaces[0].ID, Objective: "Existing Dot history", Mode: "local",
		SourceRoot: filepath.Join(dir, "source"), OutputDir: filepath.Join(dir, "output"),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "dots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(b, &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "conversations")
	b, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dots.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reopened.Snapshot()
	if len(snapshot.Conversations) != 0 || len(snapshot.Dots) != 1 || snapshot.Dots[0].ID != dot.ID {
		t.Fatalf("legacy history was lost while adding optional conversation data: %#v", snapshot)
	}
}

func conversationFolders(t *testing.T, dir string) (string, string) {
	t.Helper()
	sourceDir := filepath.Join(dir, "source")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return sourceDir, outputDir
}
