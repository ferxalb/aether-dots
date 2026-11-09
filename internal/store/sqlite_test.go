package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"aether-dots/internal/task"
)

func TestLegacyJSONIsImportedOnceAndKeptAsBackup(t *testing.T) {
	src, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := src.CreateConversation(task.CreateConversation{SpaceID: src.Snapshot().Spaces[0].ID, Name: "Imported"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.CreateConversationRun(c.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	dir := legacyDir(t, src, func(map[string]any) {})
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if len(snap.Conversations) != 1 || len(snap.Conversations[0].Messages) != 1 || len(snap.Dots) != 1 {
		t.Fatalf("legacy data was not imported: %#v", snap)
	}
	if _, err := os.Stat(filepath.Join(dir, "dots.json.migrated")); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dots.json")); !os.IsNotExist(err) {
		t.Fatal("legacy dots.json is still in place and could be re-imported")
	}
	s.Close()
	again, err := Open(dir)
	if err != nil || len(again.Snapshot().Conversations) != 1 {
		t.Fatalf("reopen after import: %v", err)
	}
}

func TestCommitsWriteOnlyChangedRows(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(task.CreateConversation{SpaceID: s.Snapshot().Spaces[0].ID, Name: "Big"})
	if err != nil {
		t.Fatal(err)
	}
	var dot task.Dot
	for i := 0; i < 300; i++ {
		dot, _, err = s.CreateConversationRun(c.ID, fmt.Sprintf("message %d", i))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetStatus(dot.ID, task.Done, "done"); err != nil {
			t.Fatal(err)
		}
	}
	var before, after int64
	changes := func() int64 {
		var n int64
		if err := s.sqlDB.QueryRow("SELECT total_changes()").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before = changes()
	if err := s.AppendEvent(dot.ID, "note", "one more event"); err != nil {
		t.Fatal(err)
	}
	after = changes()
	// One event row plus the Dot row; never proportional to the 300 runs.
	if got := after - before; got > 4 {
		t.Fatalf("appending one event changed %d rows", got)
	}
}

func TestSecondHandleSeesCommitsWithoutRewrite(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	space, err := a.CreateSpace("From A")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sp := range b.Snapshot().Spaces {
		found = found || sp.ID == space.ID
	}
	if !found {
		t.Fatal("second handle did not adopt the first handle's commit")
	}
	if _, err := b.CreateSpace("From B"); err != nil {
		t.Fatal(err)
	}
	if n := len(a.Snapshot().Spaces); n != 3 {
		t.Fatalf("spaces=%d, want 3 (merge, not overwrite)", n)
	}
}

func benchStore(b *testing.B, runs int) *Store {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	for k := 0; k < 20; k++ {
		c, err := s.CreateConversation(task.CreateConversation{SpaceID: s.Snapshot().Spaces[0].ID, Name: fmt.Sprintf("Dot %d", k)})
		if err != nil {
			b.Fatal(err)
		}
		for i := 0; i < runs; i++ {
			d, _, err := s.CreateConversationRun(c.ID, "a user request of moderate length to resemble chat")
			if err != nil {
				b.Fatal(err)
			}
			if err := s.SetStatus(d.ID, task.Done, "done"); err != nil {
				b.Fatal(err)
			}
		}
	}
	return s
}

func BenchmarkAppendEventLargeStore(b *testing.B) {
	for _, runs := range []int{10, 50, 200} {
		b.Run(fmt.Sprintf("runs_%d", runs*20), func(b *testing.B) {
			s := benchStore(b, runs)
			dot := s.Snapshot().Dots[0]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := s.AppendEvent(dot.ID, "note", "x"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLegacyJSONRewrite measures what the previous kernel did on every
// change: marshal and rewrite the whole database.
func BenchmarkLegacyJSONRewrite(b *testing.B) {
	s := benchStore(b, 100)
	snap := s.Snapshot()
	path := filepath.Join(b.TempDir(), "dots.json")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, _ := json.Marshal(snap)
		f, _ := os.Create(path)
		f.Write(raw)
		f.Sync()
		f.Close()
		os.Rename(path, path+".x")
	}
}

// dotEqual lists fields by hand for speed. If this fails, a field was added to
// task.Dot: add it to dotEqual (and to clone.go if it is a slice or pointer).
func TestDotEqualCoversEveryField(t *testing.T) {
	if n := reflect.TypeOf(task.Dot{}).NumField(); n != 16 {
		t.Fatalf("task.Dot has %d fields; update dotEqual and this count", n)
	}
}
