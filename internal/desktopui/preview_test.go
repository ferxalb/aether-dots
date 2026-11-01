package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

// TestWriteUIPreviews is an opt-in visual review helper. Set
// AETHER_UI_PREVIEW_DIR to render deterministic named Conversation and Creator
// routes without maintaining golden images in the repository.
func TestWriteUIPreviews(t *testing.T) {
	dir := os.Getenv("AETHER_UI_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set AETHER_UI_PREVIEW_DIR to write UI preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	chatApp, creatorApp := newUIPreviewApps(t)
	views := []struct {
		name string
		app  *app
	}{
		{name: "chat-dark", app: chatApp},
		{name: "creator-dark", app: creatorApp},
	}
	for _, view := range views {
		for _, appearance := range []string{"Dark", "Light"} {
			previewApp := *view.app
			previewApp.prefs.Appearance = appearance
			for _, size := range []struct {
				name          string
				width, height int
			}{{"normal", 1120, 760}, {"minimum", 760, 560}} {
				image := ui.Render(previewApp.view, size.width, size.height, 1)
				name := view.name + "-" + appearance + "-" + size.name + "-" +
					strconv.Itoa(size.width) + "x" + strconv.Itoa(size.height) + ".png"
				file := filepath.Join(dir, name)
				out, err := os.Create(file)
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(out, image); err != nil {
					out.Close()
					t.Fatal(err)
				}
				if err := out.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func newUIPreviewApps(t *testing.T) (*app, *app) {
	t.Helper()
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "Notes")
	outputDir := filepath.Join(root, "Results")
	for _, path := range []string{sourceRoot, outputDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "planning.md"), []byte("TODO: confirm launch date\nTODO: assign design owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: "Design review",
		Purpose: "Find open decisions and prepare a concise brief with source evidence.",
		Mode:    "model", SourceRoot: sourceRoot, OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	first, _, err := db.CreateConversationRun(conversation.ID, "Summarize the three open decisions in the notes.")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AppendEvent(first.ID, "source_read", "Read 4 selected text files from the approved source folder."); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendEvent(first.ID, "model_started", "Prepared a bounded draft from the selected text."); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(first.ID, task.Done, "Run finished; the local artifact was byte-verified."); err != nil {
		t.Fatal(err)
	}
	report := "# Open decisions\n\n- Confirm the launch date; no owner is recorded.\n- Assign a design reviewer before the next milestone.\n- Decide whether the export format includes source citations.\n"
	resultPath := filepath.Join(outputDir, "decision-brief.md")
	if err := os.WriteFile(resultPath, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(report))
	if err := db.AddArtifact(first.ID, task.Artifact{
		Path: resultPath, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(report)), VerifiedAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendConversationResult(conversation.ID, first.ID, report); err != nil {
		t.Fatal(err)
	}

	active, _, err := db.CreateConversationRun(conversation.ID, "Which decision needs an owner first?")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(active.ID, task.Running, "Preparing the next task."); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCheckpoint(active.ID, "composing a bounded model draft"); err != nil {
		t.Fatal(err)
	}

	chatApp := &app{
		db: db, dataDir: filepath.Join(root, "preferences"), prefs: defaultPreferences(),
		selectedSpaceID: spaceID, selectedConversationID: conversation.ID,
		runningDotID: active.ID, live: "The launch date should be assigned first so the design review can be scheduled against it.",
		cancelRun: func() {}, chatScroll: ui.ScrollState{Y: math.MaxFloat32},
	}
	chatApp.prefs.Model = "gpt-5.2"
	chatApp.refresh()
	creatorApp := *chatApp
	creatorApp.selectedConversationID = ""
	creatorApp.selectedDotID = ""
	creatorApp.runningDotID = ""
	creatorApp.live = ""
	creatorApp.cancelRun = nil
	creatorApp.creatorOpen = true
	creatorApp.dotName = "Design review"
	creatorApp.objective = conversation.Purpose
	creatorApp.mode = "model"
	creatorApp.sourceRoot = sourceRoot
	creatorApp.outputDir = outputDir
	return chatApp, &creatorApp
}
