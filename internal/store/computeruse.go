package store

import (
	"errors"
	"fmt"
	"time"

	"aether-dots/internal/task"
)

// ComputerUseToolScopes returns only the current interactive Dot's explicit
// app scopes. Routine and delegated runs never inherit this capability.
func (s *Store) ComputerUseToolScopes(dotID string) []string {
	return s.computerUseToolScopes(dotID, false)
}

// HasAnyComputerUseGrant checks whether any interactive Dot currently has a
// valid Computer Use grant. The single indexed pass is used for startup
// prewarming; per-action authorization still calls ComputerUseToolScopes.
func (s *Store) HasAnyComputerUseGrant() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversations := make(map[string]*task.Conversation, len(s.db.Conversations))
	for i := range s.db.Conversations {
		conversation := &s.db.Conversations[i]
		conversations[conversation.ID] = conversation
	}
	for i := range s.db.Dots {
		dot := &s.db.Dots[i]
		if dot.ConversationID == "" || dot.RoutineID != "" || dot.DelegatedByDotID != "" {
			continue
		}
		conversation := conversations[dot.ConversationID]
		if conversation == nil || !conversation.ComputerUseModelDisclosureAccepted ||
			conversation.ComputerUseGrantSpaceID == "" ||
			conversation.ComputerUseGrantSpaceID != conversation.SpaceID ||
			conversation.SpaceID != dot.SpaceID {
			continue
		}
		grants, err := validateComputerUseScopes(conversation.ComputerUseScopes)
		if err == nil && len(grants) > 0 {
			return true
		}
	}
	return false
}

// ComputerUseToolAuthorized rechecks the grant and active local run status at
// the point where an app action is about to execute.
func (s *Store) ComputerUseToolAuthorized(dotID, scope string) bool {
	for _, granted := range s.computerUseToolScopes(dotID, true) {
		if granted == scope {
			return true
		}
	}
	return false
}

func (s *Store) computerUseToolScopes(dotID string, requireRunning bool) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var dot *task.Dot
	for i := range s.db.Dots {
		if s.db.Dots[i].ID == dotID {
			dot = &s.db.Dots[i]
			break
		}
	}
	if dot == nil || dot.ConversationID == "" || dot.RoutineID != "" || dot.DelegatedByDotID != "" {
		return nil
	}
	if requireRunning && dot.Status != task.Running {
		return nil
	}
	ci := conversationIndex(&s.db, dot.ConversationID)
	if ci < 0 {
		return nil
	}
	conversation := s.db.Conversations[ci]
	if !conversation.ComputerUseModelDisclosureAccepted || conversation.ComputerUseGrantSpaceID == "" ||
		conversation.ComputerUseGrantSpaceID != conversation.SpaceID || conversation.SpaceID != dot.SpaceID {
		return nil
	}
	grants, err := validateComputerUseScopes(conversation.ComputerUseScopes)
	if err != nil {
		return nil
	}
	return grants
}

// RevokeAllComputerUse removes every Dot grant and model disclosure. It is
// used when the user disconnects the desktop integration.
func (s *Store) RevokeAllComputerUse() error {
	return s.change(func(db *task.Database) error {
		now := time.Now().UTC()
		for i := range db.Conversations {
			conversation := &db.Conversations[i]
			if len(conversation.ComputerUseScopes) == 0 && conversation.ComputerUseGrantSpaceID == "" && !conversation.ComputerUseModelDisclosureAccepted {
				continue
			}
			conversation.ComputerUseScopes = nil
			conversation.ComputerUseGrantSpaceID = ""
			conversation.ComputerUseModelDisclosureAccepted = false
			conversation.UpdatedAt = now
		}
		return nil
	})
}

func validateComputerUseScopes(scopes []string) ([]string, error) {
	if len(scopes) > task.MaxComputerUseScopes {
		return nil, fmt.Errorf("a Dot can be granted at most %d Computer Use scopes", task.MaxComputerUseScopes)
	}
	seen := make(map[string]bool, len(scopes))
	grants := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		switch scope {
		case task.ComputerUseScopeNotesRead, task.ComputerUseScopeMusicControl:
		default:
			return nil, errors.New("Computer Use scope is not allow-listed")
		}
		if !seen[scope] {
			seen[scope] = true
			grants = append(grants, scope)
		}
	}
	return grants, nil
}
