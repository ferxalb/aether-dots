package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

func (s *Store) CreateConversation(in task.CreateConversation) (task.Conversation, error) {
	in.SpaceID = strings.TrimSpace(in.SpaceID)
	in.Name = strings.TrimSpace(in.Name)
	in.Icon = strings.TrimSpace(in.Icon)
	in.Color = strings.TrimSpace(in.Color)
	in.Purpose = strings.TrimSpace(in.Purpose)
	in.Instructions = strings.TrimSpace(in.Instructions)
	if in.Name == "" {
		in.Name = "New Dot"
	}
	if len(in.Name) > task.MaxConversationNameBytes {
		return task.Conversation{}, fmt.Errorf("conversation name must be 1–%d bytes", task.MaxConversationNameBytes)
	}
	if len(in.Purpose) > task.MaxConversationPurposeBytes {
		return task.Conversation{}, fmt.Errorf("Dot objective must be at most %d bytes", task.MaxConversationPurposeBytes)
	}
	if len(in.Instructions) > task.MaxConversationInstructionsBytes {
		return task.Conversation{}, fmt.Errorf("working instructions must be at most %d bytes", task.MaxConversationInstructionsBytes)
	}
	if !utf8.ValidString(in.Name) || !utf8.ValidString(in.Purpose) || !utf8.ValidString(in.Instructions) {
		return task.Conversation{}, errors.New("Dot profile must be valid UTF-8 text")
	}
	if in.Icon == "" {
		in.Icon = "✦"
	}
	if in.Color == "" {
		in.Color = "#7458D8"
	}
	if err := validateProfileStyle(in.Icon, in.Color); err != nil {
		return task.Conversation{}, err
	}
	id, err := newID()
	if err != nil {
		return task.Conversation{}, err
	}
	now := time.Now().UTC()
	conversation := task.Conversation{
		ID: id, SpaceID: in.SpaceID, Name: in.Name, Icon: in.Icon, Color: in.Color,
		Purpose: in.Purpose, Instructions: in.Instructions,
		CreatedAt: now, UpdatedAt: now, Messages: []task.ConversationMessage{},
	}
	err = s.change(func(db *task.Database) error {
		if !hasSpace(db, in.SpaceID) {
			return errors.New("space not found")
		}
		db.Conversations = append(db.Conversations, conversation)
		return nil
	})
	return conversation, err
}

func (s *Store) UpdateConversationProfile(id, spaceID, name, icon, color, purpose, instructions string) error {
	spaceID = strings.TrimSpace(spaceID)
	name = strings.TrimSpace(name)
	icon = strings.TrimSpace(icon)
	color = strings.TrimSpace(color)
	purpose = strings.TrimSpace(purpose)
	instructions = strings.TrimSpace(instructions)
	if name == "" || len(name) > task.MaxConversationNameBytes {
		return fmt.Errorf("Dot name must be 1–%d bytes", task.MaxConversationNameBytes)
	}
	if len(purpose) > task.MaxConversationPurposeBytes || len(instructions) > task.MaxConversationInstructionsBytes {
		return errors.New("Dot objective or working instructions exceed the allowed length")
	}
	if !utf8.ValidString(name) || !utf8.ValidString(purpose) || !utf8.ValidString(instructions) {
		return errors.New("Dot profile is incomplete or invalid")
	}
	if err := validateProfileStyle(icon, color); err != nil {
		return err
	}
	return s.change(func(db *task.Database) error {
		i := conversationIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		if !hasSpace(db, spaceID) {
			return errors.New("Space not found")
		}
		conversation := &db.Conversations[i]
		conversation.SpaceID = spaceID
		conversation.Name = name
		conversation.Icon = icon
		conversation.Color = color
		conversation.Purpose = purpose
		conversation.Instructions = instructions
		conversation.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func validateProfileStyle(icon, color string) error {
	if icon == "" || len(icon) > 32 || !utf8.ValidString(icon) {
		return errors.New("Dot icon must contain 1–32 bytes of valid UTF-8 text")
	}
	if len(color) != 7 || color[0] != '#' {
		return errors.New("Dot color must be a six-digit hex value such as #7458D8")
	}
	for _, digit := range color[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", digit) {
			return errors.New("Dot color must be a six-digit hex value such as #7458D8")
		}
	}
	return nil
}

// CreateConversationRun atomically records a user request and the Dot run that
// will answer it using the saved profile and recent chat context.
func (s *Store) CreateConversationRun(conversationID, content string) (task.Dot, task.ConversationMessage, error) {
	if strings.TrimSpace(content) == "" {
		return task.Dot{}, task.ConversationMessage{}, errors.New("message is empty")
	}
	if len(content) > task.MaxConversationMessageBytes {
		return task.Dot{}, task.ConversationMessage{}, fmt.Errorf("message exceeds the %d-byte limit", task.MaxConversationMessageBytes)
	}
	if !utf8.ValidString(content) {
		return task.Dot{}, task.ConversationMessage{}, errors.New("message must be valid UTF-8 text")
	}
	dotID, err := newID()
	if err != nil {
		return task.Dot{}, task.ConversationMessage{}, err
	}
	messageID, err := newID()
	if err != nil {
		return task.Dot{}, task.ConversationMessage{}, err
	}
	now := time.Now().UTC()
	var dot task.Dot
	var message task.ConversationMessage
	err = s.change(func(db *task.Database) error {
		ci := conversationIndex(db, conversationID)
		if ci < 0 {
			return errors.New("conversation not found")
		}
		conversation := &db.Conversations[ci]
		if !hasSpace(db, conversation.SpaceID) {
			return errors.New("conversation space not found")
		}
		for _, existing := range db.Dots {
			if existing.ConversationID != "" && (existing.Status == task.Captured || existing.Status == task.Ready || existing.Status == task.Running) {
				return errors.New("a Dot delegation is already queued or running")
			}
		}
		parentDotID := ""
		for i := len(db.Dots) - 1; i >= 0; i-- {
			parent := db.Dots[i]
			if parent.ConversationID != conversation.ID {
				continue
			}
			if parent.SpaceID != conversation.SpaceID {
				return errors.New("saved Dot space no longer matches its task history")
			}
			parentDotID = parent.ID
			break
		}
		objective := conversationObjective(*conversation, content)
		if len(objective) > task.MaxConversationObjectiveBytes {
			return errors.New("combined Dot purpose and message exceed the objective limit")
		}
		dot = task.Dot{
			ID: dotID, SpaceID: conversation.SpaceID,
			ConversationID: conversation.ID, ParentDotID: parentDotID,
			Name: shortName(content), Objective: objective,
			Status:    task.Ready,
			CreatedAt: now, UpdatedAt: now,
			Events: []task.Event{},
		}
		appendEvent(&dot, "captured", "Chat message queued for this Dot.")
		message = task.ConversationMessage{ID: messageID, Role: "user", Content: content, DotID: dot.ID, CreatedAt: now}
		conversation.Messages = append(conversation.Messages, message)
		conversation.UpdatedAt = now
		db.Dots = append(db.Dots, dot)
		return nil
	})
	return dot, message, err
}

// AppendConversationResult stores a generated assistant response. Repeating the
// same completion is idempotent.
func (s *Store) AppendConversationResult(conversationID, dotID, report string) error {
	const maxResponseBytes = 2 * 1024 * 1024
	if len(report) == 0 || len(report) > maxResponseBytes {
		return fmt.Errorf("conversation response must be between 1 and %d bytes", maxResponseBytes)
	}
	if !utf8.ValidString(report) {
		return errors.New("conversation result is not valid UTF-8 text")
	}
	messageID, err := newID()
	if err != nil {
		return err
	}
	return s.change(func(db *task.Database) error {
		ci := conversationIndex(db, conversationID)
		if ci < 0 {
			return errors.New("conversation not found")
		}
		di := dotIndex(db, dotID)
		if di < 0 {
			return errors.New("Dot not found")
		}
		conversation, dot := &db.Conversations[ci], &db.Dots[di]
		if dot.ConversationID != conversationID {
			return errors.New("Dot does not belong to this conversation")
		}
		if dot.Status != task.Running && dot.Status != task.Done {
			return errors.New("only an active or completed Dot can save a conversation response")
		}
		for _, existing := range conversation.Messages {
			if existing.Role == "assistant" && existing.DotID == dotID {
				if existing.Content == report {
					return nil
				}
				return errors.New("a result for this Dot is already saved")
			}
		}
		conversation.Messages = append(conversation.Messages, task.ConversationMessage{
			ID: messageID, Role: "assistant",
			Content: report, DotID: dotID, CreatedAt: time.Now().UTC(),
		})
		conversation.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// RecoverInterruptedRuns marks conversation Dots that were running when the
// desktop app last exited. It assumes the desktop is the sole live writer for
// its data directory; CLI reads do not call this method.
func (s *Store) RecoverInterruptedRuns() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.db)
	if err != nil {
		return 0, err
	}
	var next task.Database
	if err := json.Unmarshal(data, &next); err != nil {
		return 0, err
	}
	count := 0
	for i := range next.Dots {
		dot := &next.Dots[i]
		if dot.ConversationID == "" || dot.Status != task.Running {
			continue
		}
		dot.Status = task.Paused
		dot.UpdatedAt = time.Now().UTC()
		if dot.Checkpoint == "" {
			dot.Checkpoint = "previous app session ended; retry this message"
		}
		appendEvent(dot, "interrupted", "Previous app session ended before the response completed.")
		count++
	}
	if count == 0 {
		return 0, nil
	}
	if err := s.persist(next); err != nil {
		return 0, err
	}
	s.db = next
	return count, nil
}

func conversationIndex(db *task.Database, id string) int {
	for i := range db.Conversations {
		if db.Conversations[i].ID == id {
			return i
		}
	}
	return -1
}

func conversationObjective(conversation task.Conversation, message string) string {
	var b strings.Builder
	b.WriteString("Dot name:\n")
	b.WriteString(conversation.Name)
	b.WriteString("\n\nDot objective:\n")
	if conversation.Purpose == "" {
		b.WriteString("No long-term objective has been set yet. Use the current request to understand the work.")
	} else {
		b.WriteString(conversation.Purpose)
	}
	if conversation.Instructions != "" {
		b.WriteString("\n\nDot working instructions:\n")
		b.WriteString(conversation.Instructions)
	}
	context := boundedConversationContext(conversation.Messages)
	if context != "" {
		b.WriteString("\n\nRecent conversation context (up to the latest 12 messages; older messages may be omitted and long entries excerpted):\n")
		b.WriteString(context)
	}
	b.WriteString("\n\nNewest user message:\n")
	b.WriteString(message)
	return b.String()
}

func boundedConversationContext(messages []task.ConversationMessage) string {
	recent := make([]task.ConversationMessage, 0, task.MaxConversationContextMessages)
	for i := len(messages) - 1; i >= 0 && len(recent) < task.MaxConversationContextMessages; i-- {
		if messages[i].Role == "user" || messages[i].Role == "assistant" {
			recent = append(recent, messages[i])
		}
	}
	if len(recent) == 0 {
		return ""
	}
	perMessage := (task.MaxConversationContextBytes - 2*(len(recent)-1)) / len(recent)
	entries := make([]string, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		entry, ok := conversationContextEntry(recent[i], perMessage)
		if ok {
			entries = append(entries, entry)
		}
	}
	return strings.Join(entries, "\n\n")
}

func conversationContextEntry(message task.ConversationMessage, maxBytes int) (string, bool) {
	label := "User request"
	if message.Role == "assistant" {
		label = "Assistant reply"
	} else if message.Role != "user" {
		return "", false
	}
	header := label + ":\n"
	separator := "\n"
	if maxBytes <= len(header)+len(separator) {
		return "", false
	}
	content := message.Content
	contentBudget := maxBytes - len(header) - len(separator)
	marker := "\n[excerpt omitted]"
	if len(content) > contentBudget {
		contentBudget -= len(marker)
		if contentBudget <= 0 {
			return "", false
		}
		content = excerptUTF8(content, contentBudget)
		content += marker
	}
	return header + content, true
}

func excerptUTF8(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	usable := maxBytes - len("…")
	first := usable * 2 / 3
	last := usable - first
	for first > 0 && !utf8.RuneStart(content[first]) {
		first--
	}
	startLast := len(content) - last
	for startLast < len(content) && !utf8.RuneStart(content[startLast]) {
		startLast++
	}
	if first == 0 || startLast >= len(content) || first >= startLast {
		end := maxBytes
		for end > 0 && !utf8.RuneStart(content[end]) {
			end--
		}
		return content[:end]
	}
	return content[:first] + "…" + content[startLast:]
}
