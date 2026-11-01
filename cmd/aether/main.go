package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
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
	dataDir := root.String("data-dir", "", "local Dot chat store folder (must appear before the command)")
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
	case "create", "capture":
		return createCommand(db, args[1:])
	case "list":
		return listCommand(db)
	case "inspect":
		return inspectCommand(db, args[1:])
	case "send":
		return sendCommand(db, args[1:])
	case "run", "resume":
		if len(args) != 2 {
			return fmt.Errorf("usage: aether %s <message-id>", args[0])
		}
		return runMessage(db, args[1])
	case "model":
		return modelCommand(args[1:])
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

func createCommand(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	spaceName := fs.String("space", "Personal", "Space name")
	name := fs.String("name", "New Dot", "Dot name")
	objective := fs.String("objective", "", "Dot's long-term objective")
	instructions := fs.String("instructions", "", "Dot's lasting working instructions")
	icon := fs.String("icon", "✦", "Dot icon")
	color := fs.String("color", "#7458D8", "Dot color")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: aether create [--name <name>] [--objective <text>] [--instructions <text>] [--space Personal]")
	}
	snapshot := db.Snapshot()
	spaceID := ""
	for _, space := range snapshot.Spaces {
		if strings.EqualFold(space.Name, *spaceName) {
			spaceID = space.ID
			break
		}
	}
	if spaceID == "" {
		return fmt.Errorf("Space %q not found; use `aether spaces create`", *spaceName)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: spaceID, Name: *name, Icon: *icon, Color: *color,
		Purpose: *objective, Instructions: *instructions,
	})
	if err == nil {
		fmt.Printf("Created Dot %q (%s)\n", conversation.Name, conversation.ID)
	}
	return err
}

func sendCommand(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	message := fs.String("message", "", "message for the Dot")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*message) == "" {
		return errors.New("usage: aether send <dot-id> --message <text>")
	}
	dot, _, err := db.CreateConversationRun(fs.Arg(0), *message)
	if err != nil {
		return err
	}
	return runMessage(db, dot.ID)
}

func runMessage(db *store.Store, id string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	generator := model.Generator{Config: model.FromEnvironment()}
	if err := engine.Run(ctx, db, id, &generator, func(delta string) { fmt.Print(delta) }); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\nReply paused. Retry the message when ready.")
			return nil
		}
		return err
	}
	fmt.Println()
	return nil
}

func listCommand(db *store.Store) error {
	snapshot := db.Snapshot()
	spaces := make(map[string]string, len(snapshot.Spaces))
	for _, space := range snapshot.Spaces {
		spaces[space.ID] = space.Name
	}
	for _, conversation := range snapshot.Conversations {
		preview := "Ready for a prompt"
		if len(conversation.Messages) > 0 {
			preview = strings.Join(strings.Fields(conversation.Messages[len(conversation.Messages)-1].Content), " ")
		}
		fmt.Printf("%s\t%-20s\t%-12s\t%s\n", conversation.ID, conversation.Name, spaces[conversation.SpaceID], preview)
	}
	return nil
}

func inspectCommand(db *store.Store, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: aether inspect <dot-id>")
	}
	snapshot := db.Snapshot()
	for _, conversation := range snapshot.Conversations {
		if conversation.ID != args[0] {
			continue
		}
		fmt.Printf("Dot %s — %s\nSpace: %s\nObjective: %s\n", conversation.ID, conversation.Name,
			spaceName(snapshot, conversation.SpaceID), conversation.Purpose)
		if conversation.Instructions != "" {
			fmt.Printf("Working instructions: %s\n", conversation.Instructions)
		}
		for _, message := range conversation.Messages {
			fmt.Printf("\n%s: %s\n", strings.Title(message.Role), message.Content)
		}
		return nil
	}
	return errors.New("Dot not found")
}

func modelCommand(args []string) error {
	if len(args) != 1 || args[0] != "test" {
		return errors.New("usage: aether model test")
	}
	config := model.FromEnvironment()
	fmt.Printf("Testing OpenAI Responses API with model %s using OPENAI_API_KEY…\n", config.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := (model.Generator{Config: config}).Test(ctx); err != nil {
		return err
	}
	fmt.Println("Model test passed.")
	return nil
}

func spaceName(db task.Database, id string) string {
	for _, space := range db.Spaces {
		if space.ID == id {
			return space.Name
		}
	}
	return "None"
}

func printHelp() {
	fmt.Print(`AETHER Dots — focused local Dot profiles and chat

Usage:
  aether [--data-dir <path>] <command>

Commands:
  create [profile options]     Create a Dot profile
  send <dot-id> --message ...  Send a message and print its reply
  list                         List Dot profiles
  inspect <dot-id>             Show a Dot profile and chat history
  run <message-id>              Retry a saved message
  spaces [list|create|rename]  Manage Spaces
  model test                   Test the OpenAI connection

Profile options:
  --name <name>
  --objective <text>
  --instructions <text>
  --icon <symbol>
  --color <hex>
  --space <name>

Dot replies use profile and chat context. The CLI does not read or write
user-selected files or folders. Set OPENAI_API_KEY for CLI model access.
`)
}
