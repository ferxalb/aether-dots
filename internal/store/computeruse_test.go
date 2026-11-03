package store

import (
	"fmt"
	"testing"

	"aether-dots/internal/task"
)

func TestHasAnyComputerUseGrantMatchesInteractiveDotAuthorization(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spaceID := db.Snapshot().Spaces[0].ID
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: spaceID, Name: "Test Dot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateConversationRun(conversation.ID, "Run a local check"); err != nil {
		t.Fatal(err)
	}
	if db.HasAnyComputerUseGrant() {
		t.Fatal("ungranted Dot appeared authorized")
	}
	if err := db.UpdateConversationProfileAndIntegrations(conversation.ID, spaceID,
		conversation.Name, conversation.Icon, conversation.Color, conversation.Purpose, conversation.Instructions,
		nil, nil, false, []string{task.ComputerUseScopeNotesRead}, true); err != nil {
		t.Fatal(err)
	}
	if !db.HasAnyComputerUseGrant() {
		t.Fatal("valid disclosed interactive Dot grant was not detected")
	}

	db.mu.Lock()
	db.db.Dots[0].RoutineID = "routine-id"
	db.mu.Unlock()
	if db.HasAnyComputerUseGrant() {
		t.Fatal("Routine Dot grant was counted for interactive prewarming")
	}

	db.mu.Lock()
	db.db.Dots[0].RoutineID = ""
	db.db.Dots[0].DelegatedByDotID = "parent-dot"
	db.mu.Unlock()
	if db.HasAnyComputerUseGrant() {
		t.Fatal("delegated Dot grant was counted for interactive prewarming")
	}

	db.mu.Lock()
	db.db.Dots[0].DelegatedByDotID = ""
	db.db.Dots[0].SpaceID = "another-space"
	db.mu.Unlock()
	if db.HasAnyComputerUseGrant() {
		t.Fatal("grant bound to a different Space was counted")
	}
}

func BenchmarkHasAnyComputerUseGrantManyDots(b *testing.B) {
	db := benchmarkComputerUseGrantStore(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !db.HasAnyComputerUseGrant() {
			b.Fatal("final Dot grant was not found")
		}
	}
}

func BenchmarkHasAnyComputerUseGrantNPlusOneBaseline(b *testing.B) {
	db := benchmarkComputerUseGrantStore(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !legacyHasAnyComputerUseGrant(db) {
			b.Fatal("final Dot grant was not found")
		}
	}
}

func benchmarkComputerUseGrantStore(count int) *Store {
	db := &Store{}
	db.db.Conversations = make([]task.Conversation, count)
	db.db.Dots = make([]task.Dot, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("conversation-%d", i)
		db.db.Conversations[i] = task.Conversation{ID: id, SpaceID: "space"}
		db.db.Dots[i] = task.Dot{ID: fmt.Sprintf("dot-%d", i), ConversationID: id, SpaceID: "space"}
	}
	last := &db.db.Conversations[count-1]
	last.ComputerUseScopes = []string{task.ComputerUseScopeNotesRead}
	last.ComputerUseGrantSpaceID = last.SpaceID
	last.ComputerUseModelDisclosureAccepted = true
	return db
}

func legacyHasAnyComputerUseGrant(db *Store) bool {
	for _, dot := range db.db.Dots {
		if len(db.ComputerUseToolScopes(dot.ID)) > 0 {
			return true
		}
	}
	return false
}
