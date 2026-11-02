package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

// DelegationTargets returns only the source profile and explicitly granted
// same-Space peers. It avoids copying every conversation message into the
// provider adapter when building a request-scoped tool catalog.
func (s *Store) DelegationTargets(parentDotID string) (task.Conversation, []task.Conversation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var parent *task.Dot
	for i := range s.db.Dots {
		if s.db.Dots[i].ID == parentDotID {
			parent = &s.db.Dots[i]
			break
		}
	}
	if parent == nil || parent.ConversationID == "" {
		return task.Conversation{}, nil, false
	}
	var source *task.Conversation
	for i := range s.db.Conversations {
		if s.db.Conversations[i].ID == parent.ConversationID {
			source = &s.db.Conversations[i]
			break
		}
	}
	if source == nil {
		return task.Conversation{}, nil, false
	}
	sourceProfile := *source
	sourceProfile.Messages = nil
	sourceProfile.DelegatedDotIDs = append([]string(nil), source.DelegatedDotIDs...)
	if len(source.DelegatedDotIDs) == 0 {
		return sourceProfile, nil, true
	}
	if len(source.DelegatedDotIDs) > task.MaxDelegatedDots {
		return sourceProfile, nil, false
	}
	granted := make(map[string]bool, len(source.DelegatedDotIDs))
	for _, id := range source.DelegatedDotIDs {
		granted[id] = true
	}
	targets := make([]task.Conversation, 0, len(granted))
	for i := range s.db.Conversations {
		peer := s.db.Conversations[i]
		if peer.SpaceID != source.SpaceID || !granted[peer.ID] || peer.ID == source.ID {
			continue
		}
		peer.Messages = nil
		peer.DelegatedDotIDs = nil
		targets = append(targets, peer)
	}
	return sourceProfile, targets, true
}

// ConversationResult reads one saved model reply without cloning the full
// local transcript store.
func (s *Store) ConversationResult(dotID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversationID := ""
	for i := range s.db.Dots {
		if s.db.Dots[i].ID == dotID {
			conversationID = s.db.Dots[i].ConversationID
			break
		}
	}
	if conversationID == "" {
		return "", false
	}
	for i := range s.db.Conversations {
		conversation := &s.db.Conversations[i]
		if conversation.ID != conversationID {
			continue
		}
		for j := len(conversation.Messages) - 1; j >= 0; j-- {
			message := conversation.Messages[j]
			if message.Role == "assistant" && message.DotID == dotID {
				return message.Content, true
			}
		}
		return "", false
	}
	return "", false
}

// validateDelegates resolves a directional grant against the current database
// and limits it to specialists in the source Dot's Space.
func validateDelegates(db *task.Database, sourceID, spaceID string, requested []string) ([]string, error) {
	if len(requested) > task.MaxDelegatedDots {
		return nil, fmt.Errorf("a Dot can be granted access to at most %d specialists", task.MaxDelegatedDots)
	}
	seen := make(map[string]bool, len(requested))
	grants := make([]string, 0, len(requested))
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if id == sourceID {
			return nil, errors.New("a Dot cannot delegate to itself")
		}
		targetIndex := conversationIndex(db, id)
		if targetIndex < 0 {
			return nil, errors.New("a delegated Dot no longer exists")
		}
		if db.Conversations[targetIndex].SpaceID != spaceID {
			return nil, errors.New("delegation is limited to Dots in the same Space")
		}
		seen[id] = true
		grants = append(grants, id)
	}
	return grants, nil
}

// CreateDelegatedConversationRun stores a typed handoff in the receiving
// specialist's own conversation. The grant is checked in the same transaction
// that creates the run, so stale tool catalogs cannot bypass a revoked grant.
func (s *Store) CreateDelegatedConversationRun(parentDotID, targetConversationID, request, constraints, expectedOutput string) (task.Dot, error) {
	request = strings.TrimSpace(request)
	constraints = strings.TrimSpace(constraints)
	expectedOutput = strings.TrimSpace(expectedOutput)
	if request == "" || len(request) > task.MaxDelegatedTaskBytes {
		return task.Dot{}, fmt.Errorf("delegated task must be between 1 and %d bytes", task.MaxDelegatedTaskBytes)
	}
	if expectedOutput == "" {
		return task.Dot{}, errors.New("delegated task must include an expected output")
	}
	if len(constraints) > task.MaxDelegatedConstraintBytes || len(expectedOutput) > task.MaxDelegatedOutputBytes {
		return task.Dot{}, errors.New("delegated constraints or expected output exceed the allowed length")
	}
	if !utf8.ValidString(request) || !utf8.ValidString(constraints) || !utf8.ValidString(expectedOutput) {
		return task.Dot{}, errors.New("delegated task fields must be valid UTF-8 text")
	}
	var delegated task.Dot
	err := s.change(func(db *task.Database) error {
		parentIndex := dotIndex(db, parentDotID)
		if parentIndex < 0 {
			return errors.New("requesting Dot no longer exists")
		}
		parent := &db.Dots[parentIndex]
		if parent.Status != task.Running || parent.ConversationID == "" {
			return errors.New("only a running conversation Dot can delegate a task")
		}
		parentConversationIndex := conversationIndex(db, parent.ConversationID)
		targetIndex := conversationIndex(db, targetConversationID)
		if parentConversationIndex < 0 || targetIndex < 0 {
			return errors.New("requesting or delegated Dot no longer exists")
		}
		parentConversation := &db.Conversations[parentConversationIndex]
		target := &db.Conversations[targetIndex]
		if target.ID == parentConversation.ID {
			return errors.New("a Dot cannot delegate to itself")
		}
		if parent.SpaceID != parentConversation.SpaceID || target.SpaceID != parentConversation.SpaceID {
			return errors.New("delegation is limited to Dots in the same Space")
		}
		if !containsString(parentConversation.DelegatedDotIDs, target.ID) {
			return errors.New("this Dot is not allowed to consult the selected specialist")
		}
		delegationCount := 0
		for _, existing := range db.Dots {
			if existing.DelegatedByDotID == parent.ID {
				delegationCount++
			}
		}
		if delegationCount >= task.MaxDelegationsPerRun {
			return fmt.Errorf("this task has reached the limit of %d specialist handoffs", task.MaxDelegationsPerRun)
		}

		var body strings.Builder
		body.WriteString("Specialist handoff from ")
		body.WriteString(parentConversation.Name)
		body.WriteString("\n\nTask:\n")
		body.WriteString(request)
		if constraints != "" {
			body.WriteString("\n\nConstraints:\n")
			body.WriteString(constraints)
		}
		if expectedOutput != "" {
			body.WriteString("\n\nExpected output:\n")
			body.WriteString(expectedOutput)
		}
		now := time.Now().UTC()
		var err error
		delegated, _, err = createConversationRunFrom(db, target.ID, body.String(), "", now, parent.ID, "", false)
		if err != nil {
			return err
		}
		childIndex := len(db.Dots) - 1
		appendEvent(&db.Dots[childIndex], "delegated_in", "A typed task was received from another Dot.")
		db.Dots[childIndex].UpdatedAt = now
		delegated = db.Dots[childIndex]
		parent = &db.Dots[parentIndex]
		parent.Checkpoint = "consulting " + shortName(target.Name)
		parent.UpdatedAt = now
		appendEvent(parent, "delegating", "Consulting the granted specialist "+target.Name+".")
		return nil
	})
	return delegated, err
}

// AppendConversationHandoffResult makes a specialist's result visible in the
// requesting Dot's transcript. It stores only the returned answer, not the
// specialist's private conversation history.
func (s *Store) AppendConversationHandoffResult(parentDotID, sourceDotID, report string) error {
	const maxResponseBytes = 2 * 1024 * 1024
	if len(report) == 0 || len(report) > maxResponseBytes || !utf8.ValidString(report) {
		return fmt.Errorf("specialist response must be valid UTF-8 text between 1 and %d bytes", maxResponseBytes)
	}
	messageID, err := newID()
	if err != nil {
		return err
	}
	return s.change(func(db *task.Database) error {
		parentIndex := dotIndex(db, parentDotID)
		sourceIndex := dotIndex(db, sourceDotID)
		if parentIndex < 0 || sourceIndex < 0 {
			return errors.New("requesting or delegated Dot no longer exists")
		}
		parent, source := &db.Dots[parentIndex], &db.Dots[sourceIndex]
		if parent.Status != task.Running || source.Status != task.Done || source.DelegatedByDotID != parent.ID {
			return errors.New("specialist result does not belong to the active handoff")
		}
		parentConversationIndex := conversationIndex(db, parent.ConversationID)
		sourceConversationIndex := conversationIndex(db, source.ConversationID)
		if parentConversationIndex < 0 || sourceConversationIndex < 0 {
			return errors.New("requesting or delegated conversation no longer exists")
		}
		parentConversation := &db.Conversations[parentConversationIndex]
		sourceConversation := &db.Conversations[sourceConversationIndex]
		if parent.SpaceID != source.SpaceID || parentConversation.SpaceID != sourceConversation.SpaceID {
			return errors.New("specialist result belongs to a different Space")
		}
		for _, existing := range parentConversation.Messages {
			if existing.Role == "handoff" && existing.DotID == parent.ID && existing.SourceDotID == source.ID {
				if existing.Content == report {
					return nil
				}
				return errors.New("a different result is already saved for this specialist handoff")
			}
		}
		parentConversation.Messages = append(parentConversation.Messages, task.ConversationMessage{
			ID: messageID, Role: "handoff", Content: report, DotID: parent.ID,
			SourceDotID: source.ID, CreatedAt: time.Now().UTC(),
		})
		parentConversation.UpdatedAt = time.Now().UTC()
		parent.UpdatedAt = time.Now().UTC()
		appendEvent(parent, "delegation_result", "Received a result from "+sourceConversation.Name+".")
		return nil
	})
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
