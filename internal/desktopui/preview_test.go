package desktopui

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

// TestWriteUIPreviews is an opt-in visual review helper. Set
// AETHER_UI_PREVIEW_DIR to render deterministic capture and result views to
// PNG files without maintaining golden images in the repository.
func TestWriteUIPreviews(t *testing.T) {
	dir := os.Getenv("AETHER_UI_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set AETHER_UI_PREVIEW_DIR to write UI preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{
		db: db, dataDir: t.TempDir(), prefs: defaultPreferences(), mode: "local",
		objective:  "Find open design decisions in these notes and prepare a concise brief with source evidence.",
		sourceRoot: "/Users/fer/Projects/Notes", outputDir: "/Users/fer/Projects/Results",
	}
	a.prefs.Appearance = "Dark"
	a.prefs.Palette = "obsidian"
	a.refresh()

	spaceID := a.selectedSpaceID
	if _, err := db.CreateDot(task.CreateDot{
		SpaceID: spaceID, Name: "Design review", Objective: a.objective, Mode: "model",
		SourceRoot: a.sourceRoot, OutputDir: a.outputDir,
	}); err != nil {
		t.Fatal(err)
	}
	dot := db.Snapshot().Dots[0]
	if err := db.AppendEvent(dot.ID, "source_read", "Read 4 selected text files from /Users/fer/Projects/Notes."); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendEvent(dot.ID, "model_started", "Sending the selected text files to OpenAI for a bounded draft."); err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Done, "Run finished; the local artifact was byte-verified."); err != nil {
		t.Fatal(err)
	}
	if err := db.AddArtifact(dot.ID, task.Artifact{
		Path:   "/Users/fer/Projects/Results/decision-brief-00000000.md",
		SHA256: "4fd997bf74c011e5ac6efb456fd10629c4ebbd2668954114c7b54270b9e953e7", Bytes: 1842,
	}); err != nil {
		t.Fatal(err)
	}
	a.selectedDotID = dot.ID
	a.prefs.Model = "gpt-5.2"
	a.refresh()
	captureApp := *a
	captureApp.selectedDotID = ""
	captureApp.settingsOpen = false
	captureApp.selectedRoutineID = ""
	lightApp := captureApp
	lightApp.sidebarCollapsed = true
	lightApp.prefs.Appearance = "Light"
	lightApp.mode = "model"

	views := []struct {
		name string
		view func(*ui.Context)
	}{
		{name: "capture", view: captureApp.view},
		{name: "capture-light-collapsed-model", view: lightApp.view},
		{name: "detail", view: a.view},
	}
	for _, size := range []struct{ width, height int }{{1120, 760}, {760, 560}} {
		for _, view := range views {
			image := ui.Render(view.view, size.width, size.height, 1)
			file := filepath.Join(dir, view.name+"-"+strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height)+".png")
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
