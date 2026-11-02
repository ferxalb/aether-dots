package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func routinesCommand(db *store.Store, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		conversations := make(map[string]string)
		for _, conversation := range db.Snapshot().Conversations {
			conversations[conversation.ID] = conversation.Name
		}
		for _, routine := range db.Snapshot().Routines {
			state := "paused"
			next := "—"
			if routine.Active {
				state = "active"
				next = routine.NextRunAt.Local().Format(time.RFC3339)
			}
			fmt.Printf("%s\t%-8s\t%-20s\t%-28s\tevery %s\tnext %s\t%s\n", routine.ID, state,
				conversations[routine.ConversationID], routine.Name,
				task.FormatRoutineInterval(routine.IntervalSeconds), next, routine.LastStatus)
		}
		return nil
	}
	switch args[0] {
	case "create":
		return createRoutineCommand(db, args[1:])
	case "update":
		return updateRoutineCommand(db, args[1:])
	case "pause", "resume":
		if len(args) != 2 {
			return fmt.Errorf("usage: aether routines %s <routine-id>", args[0])
		}
		return db.UpdateRoutineActive(args[1], args[0] == "resume")
	case "delete":
		if len(args) != 2 {
			return errors.New("usage: aether routines delete <routine-id>")
		}
		return db.DeleteRoutine(args[1])
	case "test":
		if len(args) != 2 {
			return errors.New("usage: aether routines test <routine-id>")
		}
		return testRoutineCommand(db, args[1])
	case "inspect":
		if len(args) != 2 {
			return errors.New("usage: aether routines inspect <routine-id>")
		}
		return inspectRoutineCommand(db, args[1])
	case "serve":
		if len(args) != 1 {
			return errors.New("usage: aether routines serve")
		}
		return serveRoutines(db)
	default:
		return errors.New("usage: aether routines [list|create|update|pause|resume|test|inspect|delete|serve]")
	}
}

func inspectRoutineCommand(db *store.Store, id string) error {
	snapshot := db.Snapshot()
	for _, routine := range snapshot.Routines {
		if routine.ID != id {
			continue
		}
		state := "paused"
		if routine.Active {
			state = "active"
		}
		conversationName := "Missing Dot"
		if conversation, ok := findCLIConversation(snapshot, routine.ConversationID); ok {
			conversationName = conversation.Name
		}
		next := "Paused"
		if routine.Active {
			next = routine.NextRunAt.Local().Format(time.RFC3339)
		}
		fmt.Printf("Routine %s — %s\nDot: %s\nState: %s\nInterval: %s\nNext run: %s\nLast status: %s\nInstruction: %s\n",
			routine.ID, routine.Name, conversationName, state,
			task.FormatRoutineInterval(routine.IntervalSeconds), next, routine.LastStatus, routine.Prompt)
		fmt.Println("Recent runs:")
		if len(routine.Runs) == 0 {
			fmt.Println("  No runs yet")
		}
		for _, run := range routine.Runs {
			fmt.Printf("  %s\t%s\t%s\t%s\n", run.StartedAt.Local().Format(time.RFC3339), run.Status, run.DotID, run.Message)
		}
		return nil
	}
	return errors.New("routine not found")
}

func findCLIConversation(snapshot task.Database, id string) (task.Conversation, bool) {
	for _, conversation := range snapshot.Conversations {
		if conversation.ID == id {
			return conversation, true
		}
	}
	return task.Conversation{}, false
}

func createRoutineCommand(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("routines create", flag.ContinueOnError)
	name := fs.String("name", "", "routine name")
	prompt := fs.String("prompt", "", "instruction to run on schedule")
	every := fs.Duration("every", 0, "repeat interval (minimum 5m; for example 1h or 24h)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *every <= 0 {
		return errors.New("usage: aether routines create <dot-id> --name <name> --prompt <instruction> --every <duration>")
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: fs.Arg(0), Name: *name, Prompt: *prompt,
		IntervalSeconds: int64(*every / time.Second),
	})
	if err == nil {
		fmt.Printf("Created routine %q (%s); first run at %s\n", routine.Name, routine.ID, routine.NextRunAt.Local().Format(time.RFC3339))
	}
	return err
}

func updateRoutineCommand(db *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aether routines update <routine-id> [--dot <dot-id>] [--name ...] [--prompt ...] [--every ...]")
	}
	var existing task.Routine
	for _, routine := range db.Snapshot().Routines {
		if routine.ID == args[0] {
			existing = routine
			break
		}
	}
	if existing.ID == "" {
		return errors.New("routine not found")
	}
	fs := flag.NewFlagSet("routines update", flag.ContinueOnError)
	dotID := fs.String("dot", existing.ConversationID, "owning Dot ID")
	name := fs.String("name", existing.Name, "routine name")
	prompt := fs.String("prompt", existing.Prompt, "instruction to run on schedule")
	every := fs.Duration("every", time.Duration(existing.IntervalSeconds)*time.Second, "repeat interval")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected trailing arguments")
	}
	if err := db.UpdateRoutine(existing.ID, task.CreateRoutine{
		ConversationID: *dotID, Name: *name, Prompt: *prompt,
		IntervalSeconds: int64(*every / time.Second),
	}); err != nil {
		return err
	}
	fmt.Printf("Updated routine %q (%s)\n", *name, existing.ID)
	return nil
}

func testRoutineCommand(db *store.Store, id string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := engine.RunRoutineOnceWithTaskFactory(ctx, db, id, func(dot task.Dot) engine.Generator {
		return engine.WithSpecialistDelegation(db, dot.ID, model.NewProvider(model.FromEnvironment()), nil)
	}, nil); err != nil {
		return err
	}
	snapshot := db.Snapshot()
	for i := len(snapshot.Dots) - 1; i >= 0; i-- {
		dot := snapshot.Dots[i]
		if dot.RoutineID != id {
			continue
		}
		for _, conversation := range snapshot.Conversations {
			if conversation.ID != dot.ConversationID {
				continue
			}
			for _, message := range conversation.Messages {
				if message.DotID == dot.ID && message.Role == "assistant" {
					fmt.Println(message.Content)
					return nil
				}
			}
		}
		return errors.New("routine finished without a saved response")
	}
	return errors.New("routine test run was not saved")
}

func serveRoutines(db *store.Store) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	generator := func(dot task.Dot) engine.Generator {
		return engine.WithSpecialistDelegation(db, dot.ID, model.NewProvider(model.FromEnvironment()), nil)
	}
	fmt.Println("Routine scheduler running. Press Ctrl+C to stop.")
	reported := make(map[string]string)
	err := engine.RunRoutineLoopWithTaskFactory(ctx, db, generator, 30*time.Second, func() {
		for _, routine := range db.Snapshot().Routines {
			if len(routine.Runs) == 0 {
				continue
			}
			run := routine.Runs[len(routine.Runs)-1]
			signature := run.DotID + "|" + run.Status + "|" + run.Message
			if reported[routine.ID] == signature {
				continue
			}
			reported[routine.ID] = signature
			fmt.Printf("Routine %q: %s", routine.Name, run.Status)
			if run.Message != "" {
				fmt.Printf(" — %s", run.Message)
			}
			fmt.Println()
		}
	})
	if errors.Is(err, context.Canceled) {
		fmt.Println("Routine scheduler stopped.")
		return nil
	}
	return err
}
