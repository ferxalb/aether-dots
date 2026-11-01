package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func TestVerifiedConversationReportReadsOnlyExactArtifactInsideOutputGrant(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "approved-output")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "# Verified result\n\nThe saved bytes are the assistant reply."
	path := filepath.Join(outputDir, "result.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	dot := task.Dot{
		ID: "run-1", ConversationID: "conversation-1", Status: task.Done,
		OutputDir: outputDir,
		Artifacts: []task.Artifact{{Path: path, Bytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), VerifiedAt: time.Now()}},
	}
	got, err := verifiedConversationReport(dot)
	if err != nil || got != content {
		t.Fatalf("verified result=%q err=%v", got, err)
	}

	dot.Artifacts[0].Path = filepath.Join(outsideDir, filepath.Base(path))
	if err := os.WriteFile(dot.Artifacts[0].Path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifiedConversationReport(dot); err == nil {
		t.Fatal("artifact outside the saved output grant was accepted")
	}

	dot.Artifacts[0].Path = path
	if err := os.WriteFile(path, []byte(content+" changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifiedConversationReport(dot); err == nil {
		t.Fatal("artifact whose exact bytes changed after engine verification was accepted")
	}
}

func TestSendConversationMessageCreatesOneRunAndPersistsRealResult(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "notes")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "decisions.md"), []byte("TODO: confirm the launch date"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Decision helper", Purpose: "Find open decisions",
		Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, selectedSpaceID: conversation.SpaceID, selectedConversationID: conversation.ID}
	a.refresh()
	a.messageDraft = "List unresolved decisions"
	a.sendConversationMessage()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := db.Snapshot()
		if len(snapshot.Dots) == 1 && snapshot.Dots[0].Status == task.Done && len(snapshot.Conversations[0].Messages) == 2 {
			if snapshot.Conversations[0].Messages[0].Role != "user" || snapshot.Conversations[0].Messages[1].Role != "assistant" {
				t.Fatalf("conversation did not persist user request and actual result: %#v", snapshot.Conversations[0].Messages)
			}
			if snapshot.Conversations[0].Messages[1].Content == "" || len(snapshot.Dots[0].Artifacts) != 1 {
				t.Fatalf("successful run has no verified report: dot=%#v messages=%#v", snapshot.Dots[0], snapshot.Conversations[0].Messages)
			}
			return
		}
		if len(snapshot.Dots) > 0 && (snapshot.Dots[0].Status == task.Failed || snapshot.Dots[0].Status == task.WaitingInput) {
			t.Fatalf("headless local delegation failed: %#v", snapshot.Dots[0])
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delegation did not persist a verified result before timeout: %#v", db.Snapshot())
}
