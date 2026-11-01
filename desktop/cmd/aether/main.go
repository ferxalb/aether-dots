package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"aether-dots/desktop/internal/engine"
	"aether-dots/desktop/internal/model"
	"aether-dots/desktop/internal/store"
	"aether-dots/desktop/internal/task"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aether:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	root := flag.NewFlagSet("aether", flag.ContinueOnError)
	root.SetOutput(os.Stderr)
	dataDir := root.String("data-dir", "", "local task store folder (must appear before the command)")
	if err := root.Parse(args); err != nil {
		return err
	}
	args = root.Args()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printHelp()
		return nil
	}
	if *dataDir == "" {
		var err error
		*dataDir, err = store.DefaultDataDir()
		if err != nil {
			return err
		}
	}
	db, err := store.Open(*dataDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "spaces":
		return spacesCommand(db, args[1:])
	case "capture":
		return captureCommand(db, args[1:])
	case "list":
		return listCommand(db)
	case "inspect":
		return inspectCommand(db, args[1:])
	case "mode":
		if len(args) != 3 {
			return errors.New("usage: aether mode <dot-id> local|model")
		}
		dot, err := findDot(db.Snapshot(), args[1])
		if err != nil {
			return err
		}
		return db.SetMode(dot.ID, args[2])
	case "run", "resume":
		if len(args) != 2 {
			return fmt.Errorf("usage: aether %s <dot-id>", args[0])
		}
		dot, err := findDot(db.Snapshot(), args[1])
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		generator := model.Generator{Config: model.FromEnvironment()}
		err = engine.Run(ctx, db, dot.ID, &generator, func(delta string) { fmt.Print(delta) })
		if errors.Is(err, context.Canceled) {
			fmt.Println("\nRun paused and checkpointed.")
			return nil
		}
		if err != nil {
			return err
		}
		updated, _ := db.GetDot(dot.ID)
		fmt.Printf("\nVerified output: %s\n", updated.Artifacts[len(updated.Artifacts)-1].Path)
		return nil
	case "model":
		return modelCommand(args[1:])
	case "routine":
		return routineCommand(db, args[1:])
	default:
		return fmt.Errorf("unknown command %q (try `aether help`)", args[0])
	}
}

func spacesCommand(db *store.Store, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		for _, space := range db.Snapshot().Spaces {
			fmt.Printf("%s\t%s\n", space.ID, space.Name)
		}
		return nil
	}
	switch args[0] {
	case "create":
		if len(args) != 2 {
			return errors.New("usage: aether spaces create <name>")
		}
		space, err := db.CreateSpace(args[1])
		if err == nil {
			fmt.Printf("Created %s (%s)\n", space.Name, space.ID)
		}
		return err
	case "rename":
		if len(args) != 3 {
			return errors.New("usage: aether spaces rename <space-id> <new-name>")
		}
		return db.RenameSpace(args[1], args[2])
	default:
		return errors.New("usage: aether spaces [list|create|rename]")
	}
}

func captureCommand(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	spaceName := fs.String("space", "Personal", "Space name")
	name := fs.String("name", "", "short Dot name")
	objective := fs.String("objective", "", "task outcome")
	mode := fs.String("mode", "local", "local (no model) or model")
	sourceDir := fs.String("source", "", "explicit local source folder")
	outputDir := fs.String("output", "", "explicit local output folder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*objective) == "" || *sourceDir == "" || *outputDir == "" {
		return errors.New("usage: aether capture --objective <task> --source <folder> --output <folder> [--space Personal] [--mode local|model]")
	}
	snapshot := db.Snapshot()
	var selected *task.Space
	for i := range snapshot.Spaces {
		if strings.EqualFold(snapshot.Spaces[i].Name, *spaceName) {
			selected = &snapshot.Spaces[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("Space %q not found; use `aether spaces create`", *spaceName)
	}
	sourceAbs, err := existingFolder(*sourceDir)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	outputAbs, err := existingFolder(*outputDir)
	if err != nil {
		return fmt.Errorf("output: %w", err)
	}
	dot, err := db.CreateDot(task.CreateDot{SpaceID: selected.ID, Name: *name, Objective: *objective, Mode: *mode, SourceRoot: sourceAbs, OutputDir: outputAbs})
	if err == nil {
		fmt.Printf("Captured %s (%s)\n", dot.Name, dot.ID)
		fmt.Printf("Run with: aether run %s\n", dot.ID)
	}
	return err
}

func listCommand(db *store.Store) error {
	snapshot := db.Snapshot()
	spaces := make(map[string]string, len(snapshot.Spaces))
	for _, space := range snapshot.Spaces {
		spaces[space.ID] = space.Name
	}
	for _, dot := range snapshot.Dots {
		fmt.Printf("%s\t%-18s\t%-10s\t%s\n", dot.ID, dot.Status, spaces[dot.SpaceID], dot.Name)
	}
	return nil
}

func inspectCommand(db *store.Store, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: aether inspect <dot-id>")
	}
	dot, err := findDot(db.Snapshot(), args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Dot %s — %s\nStatus: %s | mode: %s | Space: %s\nObjective: %s\nSource: %s (local only)\nOutput folder: %s\nCheckpoint: %s\n",
		dot.ID, dot.Name, dot.Status, dot.Mode, spaceName(db.Snapshot(), dot.SpaceID), dot.Objective, dot.SourceRoot, dot.OutputDir, dot.Checkpoint)
	fmt.Println("\nEvent timeline:")
	for _, event := range dot.Events {
		fmt.Printf("%03d  %s  %s  %s\n", event.Sequence, event.Timestamp.Format(time.RFC3339), event.Type, event.Message)
	}
	fmt.Println("\nVerified artifacts:")
	for _, artifact := range dot.Artifacts {
		fmt.Printf("%s\n  SHA-256 %s  (%d bytes; verified %s)\n", artifact.Path, artifact.SHA256, artifact.Bytes, artifact.VerifiedAt.Format(time.RFC3339))
	}
	return nil
}

func modelCommand(args []string) error {
	if len(args) != 1 || args[0] != "test" {
		return errors.New("usage: aether model test")
	}
	config := model.FromEnvironment()
	fmt.Printf("Testing %s via %s (%s) using key from %s…\n", config.API, config.EndpointLabel(), config.Model, config.KeyEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := (model.Generator{Config: config}).Test(ctx); err != nil {
		return err
	}
	fmt.Println("Model test passed.")
	return nil
}

func routineCommand(db *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aether routine [list|promote|edit|run]")
	}
	switch args[0] {
	case "list":
		for _, r := range db.Snapshot().Routines {
			fmt.Printf("%s\tv%d\t%s\t(read %s; write %s)\n", r.ID, r.Version, r.Name, r.SourceRoot, r.OutputDir)
		}
		return nil
	case "promote":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: aether routine promote <dot-id> [name]")
		}
		name := ""
		if len(args) == 3 {
			name = args[2]
		}
		r, err := db.PromoteRoutine(args[1], name)
		if err == nil {
			fmt.Printf("Routine saved: %s (%s), version %d\n", r.Name, r.ID, r.Version)
		}
		return err
	case "edit":
		fs := flag.NewFlagSet("routine edit", flag.ContinueOnError)
		name := fs.String("name", "", "editable routine label")
		objective := fs.String("objective", "", "editable task objective")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: aether routine edit --name <name> --objective <objective> <routine-id>")
		}
		return db.EditRoutine(fs.Arg(0), *name, *objective)
	case "run":
		if len(args) != 2 {
			return errors.New("usage: aether routine run <routine-id>")
		}
		snapshot := db.Snapshot()
		spaceID := ""
		for _, space := range snapshot.Spaces {
			if space.Name == "Personal" {
				spaceID = space.ID
				break
			}
		}
		if spaceID == "" && len(snapshot.Spaces) > 0 {
			spaceID = snapshot.Spaces[0].ID
		}
		dot, err := db.DotFromRoutine(args[1], spaceID)
		if err != nil {
			return err
		}
		fmt.Printf("Created Dot %s with the Routine's original local grants.\n", dot.ID)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		generator := model.Generator{Config: model.FromEnvironment()}
		return engine.Run(ctx, db, dot.ID, &generator, func(delta string) { fmt.Print(delta) })
	default:
		return errors.New("usage: aether routine [list|promote|edit|run]")
	}
}

func existingFolder(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("path is not an available folder")
	}
	return resolved, nil
}

func findDot(db task.Database, prefix string) (task.Dot, error) {
	for _, dot := range db.Dots {
		if dot.ID == prefix || strings.HasPrefix(dot.ID, prefix) {
			return dot, nil
		}
	}
	return task.Dot{}, fmt.Errorf("Dot %q not found", prefix)
}

func spaceName(db task.Database, id string) string {
	for _, space := range db.Spaces {
		if space.ID == id {
			return space.Name
		}
	}
	return "(missing)"
}

func printHelp() {
	fmt.Print(`AETHER Dots local task runtime

Usage: aether [--data-dir <folder>] <command>

Commands:
  spaces [list|create|rename]  Manage persisted Spaces
  capture --objective ... --source <folder> --output <folder> [--mode local|model]
  list                        List durable Dots
  inspect <dot-id>            Show status, evidence and event timeline
  mode <dot-id> local|model   Change a Dot's execution mode
  run <dot-id>                Run a captured Dot
  resume <dot-id>             Resume/retry a paused or failed Dot
  model test                  Test the OpenAI-compatible endpoint
  routine [list|promote|edit|run]

Model settings (optional): AETHER_MODEL_BASE_URL, AETHER_MODEL_NAME,
AETHER_MODEL_API (default responses), AETHER_MODEL_API_KEY_ENV, and the named
key variable (default OPENAI_API_KEY). The API key is read from the process
environment and is never saved.
`)
}
