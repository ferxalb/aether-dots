package remote

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

const maxReceipts = 500

var commandIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

type ReceiptStore struct {
	mu   sync.Mutex
	path string
	ids  []string
	seen map[string]struct{}
}

func OpenReceiptStore(dataDir string) (*ReceiptStore, error) {
	store := &ReceiptStore{path: filepath.Join(dataDir, "mobile-command-receipts.json"), seen: make(map[string]struct{})}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, errors.New("could not read the mobile command receipt store")
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil || len(ids) > maxReceipts {
		return nil, errors.New("the mobile command receipt store is invalid")
	}
	for _, id := range ids {
		if !commandIDPattern.MatchString(id) {
			return nil, errors.New("the mobile command receipt store is invalid")
		}
		if _, duplicate := store.seen[id]; duplicate {
			continue
		}
		store.ids = append(store.ids, id)
		store.seen[id] = struct{}{}
	}
	return store, nil
}

func (s *ReceiptStore) Seen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[id]
	return ok
}

func (s *ReceiptStore) Record(id string) error {
	if !commandIDPattern.MatchString(id) {
		return errors.New("invalid mobile command id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.seen[id]; exists {
		return nil
	}
	next := append(append([]string(nil), s.ids...), id)
	if len(next) > maxReceipts {
		next = append([]string(nil), next[len(next)-maxReceipts:]...)
	}
	data, err := json.Marshal(next)
	if err != nil {
		return errors.New("could not encode the mobile command receipt")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".mobile-receipts-*.tmp")
	if err != nil {
		return errors.New("could not save the mobile command receipt")
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return errors.New("could not save the mobile command receipt")
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return errors.New("could not save the mobile command receipt")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return errors.New("could not save the mobile command receipt")
	}
	if err := tmp.Close(); err != nil {
		return errors.New("could not save the mobile command receipt")
	}
	if err := os.Rename(name, s.path); err != nil {
		return errors.New("could not save the mobile command receipt")
	}
	if len(s.ids) >= maxReceipts {
		delete(s.seen, s.ids[0])
	}
	s.ids = next
	s.seen[id] = struct{}{}
	return nil
}
