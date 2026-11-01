package desktopui

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

// TestWriteUIPreviews is an opt-in visual review helper. Set
// AETHER_UI_PREVIEW_DIR to render the main chat, Dot profile, and welcome page.
func TestWriteUIPreviews(t *testing.T) {
	dir := os.Getenv("AETHER_UI_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set AETHER_UI_PREVIEW_DIR to write UI preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	chatApp, profileApp, welcomeApp := newUIPreviewApps(t)
	views := []struct {
		name string
		app  *app
	}{
		{name: "chat", app: chatApp},
		{name: "profile", app: profileApp},
		{name: "welcome", app: welcomeApp},
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

func newUIPreviewApps(t *testing.T) (*app, *app, *app) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: "Decision partner", Icon: "◈", Color: "#1D69C8",
		Purpose:      "Turn complicated discussions into short, useful decisions.",
		Instructions: "Use plain language. State what is known and what still needs a decision.",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, _, err := db.CreateConversationRun(conversation.ID, "Summarize the three open decisions.")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(first.ID, task.Running, "Reply started."); err != nil {
		t.Fatal(err)
	}
	reply := "Three decisions remain: choose the launch date, assign a design reviewer, and confirm the export format."
	if err := db.AppendConversationResult(conversation.ID, first.ID, reply); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(first.ID, task.Done, "Reply saved."); err != nil {
		t.Fatal(err)
	}

	active, _, err := db.CreateConversationRun(conversation.ID, "Which decision should be handled first?")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(active.ID, task.Running, "Preparing a reply."); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCheckpoint(active.ID, "generating response"); err != nil {
		t.Fatal(err)
	}

	chatApp := &app{
		db: db, dataDir: filepath.Join(root, "preferences"), prefs: defaultPreferences(),
		selectedSpaceID: spaceID, selectedConversationID: conversation.ID,
		runningDotID: active.ID, live: "The launch date should come first, so the review can be planned against it.",
		cancelRun: func() {}, chatScroll: ui.ScrollState{Y: math.MaxFloat32},
	}
	chatApp.prefs.Model = "gpt-6.1-sol"
	chatApp.refresh()
	profileApp := *chatApp
	profileApp.cancelRun = nil
	profileApp.runningDotID = ""
	profileApp.live = ""
	profileApp.openConversationProfile()
	welcomeDB, err := store.Open(filepath.Join(root, "welcome-store"))
	if err != nil {
		t.Fatal(err)
	}
	welcomeApp := &app{db: welcomeDB, dataDir: filepath.Join(root, "preferences"), prefs: defaultPreferences()}
	welcomeApp.refresh()
	return chatApp, &profileApp, welcomeApp
}
