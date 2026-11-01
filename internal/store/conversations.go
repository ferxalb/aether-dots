package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/source"
	"aether-dots/internal/task"
)

func (s *Store) CreateConversation(in task.CreateConversation) (task.Conversation, error) {
	in.SpaceID = strings.TrimSpace(in.SpaceID)
	in.Name = strings.TrimSpace(in.Name)
	in.Purpose = strings.TrimSpace(in.Purpose)
	in.Mode = strings.TrimSpace(in.Mode)
	if in.Name == "" || len(in.Name) > task.MaxConversationNameBytes {
		return task.Conversation{}, fmt.Errorf("conversation name must be 1–%d bytes", task.MaxConversationNameBytes)
	}
	if in.Purpose == "" || len(in.Purpose) > task.MaxConversationPurposeBytes {
		return task.Conversation{}, fmt.Errorf("Dot purpose must be 1–%d bytes", task.MaxConversationPurposeBytes)
	}
	if !utf8.ValidString(in.Name) || !utf8.ValidString(in.Purpose) {
		return task.Conversation{}, errors.New("Dot name and purpose must be valid UTF-8 text")
	}
	if in.Mode != "local" && in.Mode != "model" {
		return task.Conversation{}, errors.New("mode must be local or model")
	}
	if strings.TrimSpace(in.SourceRoot) == "" || strings.TrimSpace(in.OutputDir) == "" {
		return task.Conversation{}, errors.New("choose both a source folder and an output folder before creating a Dot")
	}
	if err := validateConversationScopes(in.SourceRoot, in.OutputDir); err != nil {
		return task.Conversation{}, err
	}
	id, err := newID()
	if err != nil {
		return task.Conversation{}, err
	}
	now := time.Now().UTC()
	conversation := task.Conversation{
		ID: id, SpaceID: in.SpaceID, Name: in.Name, Purpose: in.Purpose,
		Mode: in.Mode, SourceRoot: in.SourceRoot, OutputDir: in.OutputDir,
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

func validateConversationScopes(sourceRoot, outputDir string) error {
	sourcePath, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve selected source folder: %w", err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || !sourceInfo.IsDir() {
		return errors.New("source must be an existing folder selected by the user")
	}
	outputPath, err := filepath.EvalSymlinks(outputDir)
	if err != nil {
		return fmt.Errorf("resolve selected output folder: %w", err)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil || !outputInfo.IsDir() {
		return errors.New("output must be an existing folder selected by the user")
	}
	relative, err := filepath.Rel(sourcePath, outputPath)
	if err != nil {
		return fmt.Errorf("compare source and output folders: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))) {
		return errors.New("choose an output folder outside the selected source folder")
	}
	return nil
}

// CreateConversationRun atomically records a user request and the real Dot run
// that will execute it. The conversation's saved purpose, mode, and local path
// grants remain fixed for every child run.
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
		if conversation.Mode != "local" && conversation.Mode != "model" {
			return errors.New("conversation has an invalid execution mode")
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
			if parent.SpaceID != conversation.SpaceID || parent.SourceRoot != conversation.SourceRoot || parent.OutputDir != conversation.OutputDir {
				return errors.New("saved conversation grants no longer match its child Dot history")
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
			Status: task.Ready, Mode: conversation.Mode,
			SourceRoot: conversation.SourceRoot, OutputDir: conversation.OutputDir,
			CreatedAt: now, UpdatedAt: now,
			Events: []task.Event{}, Artifacts: []task.Artifact{},
		}
		appendEvent(&dot, "captured", "Chat delegation queued with this Dot's saved local source and output grants.")
		message = task.ConversationMessage{ID: messageID, Role: "user", Content: content, DotID: dot.ID, CreatedAt: now}
		conversation.Messages = append(conversation.Messages, message)
		conversation.UpdatedAt = now
		db.Dots = append(db.Dots, dot)
		return nil
	})
	return dot, message, err
}

// AppendConversationResult stores only the exact bytes from a verified artifact
// linked to a completed child Dot. Repeating the same completion is idempotent.
func (s *Store) AppendConversationResult(conversationID, dotID, report string) error {
	if len(report) == 0 || len(report) > source.MaxOutputBytes {
		return fmt.Errorf("conversation result must be between 1 and %d bytes", source.MaxOutputBytes)
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
		if dot.Status != task.Done || len(dot.Artifacts) == 0 {
			return errors.New("only a completed Dot with a verified artifact can be saved as a conversation result")
		}
		for _, existing := range conversation.Messages {
			if existing.Role == "assistant" && existing.DotID == dotID {
				if existing.Content == report {
					return nil
				}
				return errors.New("a result for this Dot is already saved")
			}
		}
		sum := sha256.Sum256([]byte(report))
		hash := hex.EncodeToString(sum[:])
		matched := false
		for _, artifact := range dot.Artifacts {
			if artifact.Bytes == int64(len(report)) && artifact.SHA256 == hash {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("conversation result bytes do not match a verified Dot artifact")
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
			dot.Checkpoint = "previous app session ended; resume to re-read the selected local source"
		}
		appendEvent(dot, "interrupted", "Previous app session ended; resume to re-read the selected local source.")
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
	b.WriteString("Dot purpose:\n")
	b.WriteString(conversation.Purpose)
	if conversation.Mode == "model" {
		context := boundedConversationContext(conversation.Messages)
		if context != "" {
			b.WriteString("\n\nRecent conversation context (up to the latest 6 messages; older messages may be omitted and long entries excerpted):\n")
			b.WriteString(context)
		}
	}
	b.WriteString("\n\nNewest delegation:\n")
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
		label = "Assistant result from a verified artifact"
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
