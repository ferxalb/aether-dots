package main

import (
	"errors"
	"testing"

	"aether-dots/internal/license"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func TestFreeTierCLIGates(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	org := license.OrganizationID
	t.Cleanup(func() { license.OrganizationID = org })
	license.OrganizationID = "11111111-2222-3333-4444-555555555555"

	for i := 0; i < license.FreeAgentLimit; i++ {
		if err := createCommand(db, []string{"--name", "Agent"}); err != nil {
			t.Fatalf("Free agent %d refused: %v", i+1, err)
		}
	}
	if err := createCommand(db, []string{"--name", "One too many"}); !errors.Is(err, license.ErrProRequired) {
		t.Fatalf("agent %d on Free: %v", license.FreeAgentLimit+1, err)
	}
	if n := len(db.Snapshot().Conversations); n != license.FreeAgentLimit {
		t.Fatalf("%d agents exist, want %d", n, license.FreeAgentLimit)
	}

	id := db.Snapshot().Conversations[0].ID
	if err := routinesCommand(db, []string{"create", "--name", "Daily", "--prompt", "Summarize", "--every", "24h", id}); !errors.Is(err, license.ErrProRequired) {
		t.Fatalf("routine on Free: %v", err)
	}
	if err := githubCommand(db, []string{"write-grant", "--allow-publish", id, "101:comment"}); !errors.Is(err, license.ErrProRequired) {
		t.Fatalf("write grant on Free: %v", err)
	}
	if len(db.Snapshot().Routines) != 0 {
		t.Fatal("a routine was created on Free")
	}

	license.OrganizationID = ""
	if err := createCommand(db, []string{"--name", "Unlicensed build"}); err != nil {
		t.Fatalf("build without licensing limited agents: %v", err)
	}
}

func TestCreateFromTriageTemplate(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := createCommand(db, []string{"--template", "triage", "--name", "Triage: alpha"}); err != nil {
		t.Fatal(err)
	}
	conv := db.Snapshot().Conversations[0]
	if conv.Name != "Triage: alpha" || conv.Instructions != task.TriageTemplate.Instructions || conv.Icon != task.TriageTemplate.Icon {
		t.Fatalf("agent = %+v", conv)
	}
	if err := createCommand(db, []string{"--template", "unknown"}); err == nil {
		t.Fatal("accepted an unknown template")
	}
}
