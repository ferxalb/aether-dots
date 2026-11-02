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

func (s *Store) UpdateConversationProfile(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs ...[]string) error {
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions, delegatedDotIDs, nil, nil)
}

// UpdateConversationProfileAndGitHub saves the Dot profile and its independent
// repository grants in one transaction.
func (s *Store) UpdateConversationProfileAndGitHub(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs []string, githubRepositoryIDs []int64, disclosureAccepted bool) error {
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions, [][]string{delegatedDotIDs}, &githubRepositoryIDs, &disclosureAccepted)
}

func (s *Store) updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs [][]string, githubRepositoryIDs *[]int64, disclosureAccepted *bool) error {
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
		previousSpaceID := conversation.SpaceID
		requestedDelegates := conversation.DelegatedDotIDs
		if len(delegatedDotIDs) > 0 {
			requestedDelegates = delegatedDotIDs[0]
		}
		grants, err := validateDelegates(db, id, spaceID, requestedDelegates)
		if err != nil {
			return err
		}
		if previousSpaceID != spaceID && githubRepositoryIDs != nil && len(*githubRepositoryIDs) > 0 {
			return errors.New("save the Dot's new Space before granting GitHub repositories again")
		}
		if githubRepositoryIDs != nil {
			grants, err := validateGitHubRepositoryGrants(db, *githubRepositoryIDs)
			if err != nil {
				return err
			}
			if len(grants) > 0 && (disclosureAccepted == nil || !*disclosureAccepted) {
				return errors.New("acknowledge that selected GitHub data may be sent to the configured model before granting access")
			}
			conversation.GitHubRepositoryIDs = grants
			conversation.GitHubGrantSpaceID = ""
			conversation.GitHubModelDisclosureAccepted = len(grants) > 0 && *disclosureAccepted
			if len(grants) > 0 {
				conversation.GitHubGrantSpaceID = spaceID
			}
		} else if previousSpaceID != spaceID {
			conversation.GitHubRepositoryIDs = nil
			conversation.GitHubGrantSpaceID = ""
			conversation.GitHubModelDisclosureAccepted = false
			disableConversationGitHubRoutines(db, conversation.ID)
		}
		conversation.DelegatedDotIDs = grants
		conversation.SpaceID = spaceID
		conversation.Name = name
		conversation.Icon = icon
		conversation.Color = color
		conversation.Purpose = purpose
		conversation.Instructions = instructions
		if len(conversation.GitHubRepositoryIDs) == 0 {
			disableConversationGitHubRoutines(db, conversation.ID)
		}
		conversation.UpdatedAt = time.Now().UTC()
		for peerIndex := range db.Conversations {
			peer := &db.Conversations[peerIndex]
			if peer.ID == conversation.ID || peer.SpaceID == spaceID || len(peer.DelegatedDotIDs) == 0 {
				continue
			}
			remaining := peer.DelegatedDotIDs[:0]
			for _, delegatedID := range peer.DelegatedDotIDs {
				if delegatedID != conversation.ID {
					remaining = append(remaining, delegatedID)
				}
			}
			peer.DelegatedDotIDs = remaining
		}
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
// will answer it using the saved profile and conversation history.
func (s *Store) CreateConversationRun(conversationID, content string) (task.Dot, task.ConversationMessage, error) {
	var dot task.Dot
	var message task.ConversationMessage
	err := s.change(func(db *task.Database) error {
		var err error
		dot, message, err = createConversationRun(db, conversationID, content, "", time.Now().UTC())
		return err
	})
	return dot, message, err
}

func createConversationRun(db *task.Database, conversationID, content, routineID string, now time.Time) (task.Dot, task.ConversationMessage, error) {
	return createConversationRunFrom(db, conversationID, content, routineID, now, "", "", true)
}

func createConversationRunFrom(db *task.Database, conversationID, content, routineID string, now time.Time, sourceDotID, parentDotIDOverride string, allowSkillSelection bool) (task.Dot, task.ConversationMessage, error) {
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
	var dot task.Dot
	var message task.ConversationMessage
	ci := conversationIndex(db, conversationID)
	if ci < 0 {
		return task.Dot{}, task.ConversationMessage{}, errors.New("conversation not found")
	}
	conversation := &db.Conversations[ci]
	if !hasSpace(db, conversation.SpaceID) {
		return task.Dot{}, task.ConversationMessage{}, errors.New("conversation space not found")
	}
	for _, existing := range db.Dots {
		if existing.Status != task.Captured && existing.Status != task.Ready && existing.Status != task.Running {
			continue
		}
		if existing.ConversationID == conversationID || (sourceDotID == "" && existing.DelegatedByDotID == "") {
			return task.Dot{}, task.ConversationMessage{}, ErrDotAlreadyActive
		}
	}
	parentDotID := parentDotIDOverride
	if parentDotID == "" {
		for i := len(db.Dots) - 1; i >= 0; i-- {
			parent := db.Dots[i]
			if parent.ConversationID != conversation.ID {
				continue
			}
			if parent.SpaceID != conversation.SpaceID {
				return task.Dot{}, task.ConversationMessage{}, errors.New("saved Dot space no longer matches its task history")
			}
			parentDotID = parent.ID
			break
		}
	}
	selected, hasSkill := task.Skill{}, false
	if allowSkillSelection {
		selected, hasSkill = selectSkill(db.Skills, content)
	}
	// Keep the durable Dot record compact and provider-neutral. The engine
	// assembles the complete conversation from saved messages when it runs.
	objective := conversationObjectiveBase(*conversation, content, selected, hasSkill)
	if len(objective) > task.MaxConversationObjectiveBytes {
		return task.Dot{}, task.ConversationMessage{}, errors.New("combined Dot purpose, skill, and message exceed the objective limit")
	}
	dot = task.Dot{
		ID: dotID, SpaceID: conversation.SpaceID,
		ConversationID: conversation.ID, ConversationContextDeferred: true, ParentDotID: parentDotID,
		DelegatedByDotID: sourceDotID,
		Name:             shortName(content), Objective: objective,
		Status: task.Ready, CreatedAt: now, UpdatedAt: now,
		Events: []task.Event{}, RoutineID: routineID,
	}
	if hasSkill {
		dot.SkillID = selected.ID
	}
	appendEvent(&dot, "captured", "Chat message queued for this Dot.")
	message = task.ConversationMessage{ID: messageID, Role: "user", Content: content, DotID: dot.ID, SourceDotID: sourceDotID, CreatedAt: now}
	conversation.Messages = append(conversation.Messages, message)
	conversation.UpdatedAt = now
	db.Dots = append(db.Dots, dot)
	return dot, message, nil
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
	return conversationObjectiveWithSkill(conversation, message, task.Skill{}, false)
}

func conversationObjectiveWithSkill(conversation task.Conversation, message string, skill task.Skill, hasSkill bool) string {
	return conversationObjectiveWithContext(conversation, message, skill, hasSkill,
		boundedConversationContext(conversation.Messages))
}

func conversationObjectiveBase(conversation task.Conversation, message string, skill task.Skill, hasSkill bool) string {
	return conversationObjectiveWithContext(conversation, message, skill, hasSkill, "")
}

func conversationObjectiveWithContext(conversation task.Conversation, message string, skill task.Skill, hasSkill bool, context string) string {
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
	if hasSkill {
		b.WriteString("\n\nUse this explicitly selected skill for this turn only. Its instructions do not grant access to tools, files, folders, apps, or accounts. Follow its approval boundary and state when required inputs are missing.\n")
		b.WriteString("\nSkill: ")
		b.WriteString(skill.Name)
		if skill.Description != "" {
			b.WriteString("\nDescription: ")
			b.WriteString(skill.Description)
		}
		b.WriteString("\nWhen to use: ")
		b.WriteString(skill.WhenToUse)
		b.WriteString("\nRequired inputs: ")
		b.WriteString(skill.Inputs)
		b.WriteString("\nSteps: ")
		b.WriteString(skill.Steps)
		b.WriteString("\nValidation: ")
		b.WriteString(skill.Validation)
		b.WriteString("\nExpected output: ")
		b.WriteString(skill.Output)
		b.WriteString("\nApproval boundary: ")
		b.WriteString(skill.Approval)
	}
	if context != "" {
		b.WriteString("\n\nRecent conversation context (up to the latest 12 messages; older messages may be omitted and long entries excerpted):\n")
		b.WriteString(context)
	}
	b.WriteString("\n\nNewest user message:\n")
	b.WriteString(message)
	return b.String()
}

// ConversationObjective assembles the provider prompt for a saved chat turn
// from its durable profile and every prior conversation message. Provider
// selection therefore never changes or truncates stored chat data.
func (s *Store) ConversationObjective(dotID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var dot *task.Dot
	for i := range s.db.Dots {
		if s.db.Dots[i].ID == dotID {
			dot = &s.db.Dots[i]
			break
		}
	}
	if dot == nil || dot.ConversationID == "" {
		return "", errors.New("Dot conversation not found")
	}
	if !dot.ConversationContextDeferred {
		if len(dot.Objective) > task.MaxConversationObjectiveBytes {
			return "", errors.New("saved Dot prompt exceeds the local prompt safety limit")
		}
		return dot.Objective, nil
	}
	var conversation *task.Conversation
	for i := range s.db.Conversations {
		if s.db.Conversations[i].ID == dot.ConversationID {
			conversation = &s.db.Conversations[i]
			break
		}
	}
	if conversation == nil {
		return "", errors.New("saved Dot conversation not found")
	}
	messageIndex := -1
	for i := range conversation.Messages {
		message := conversation.Messages[i]
		if message.DotID == dotID && message.Role == "user" {
			messageIndex = i
			break
		}
	}
	if messageIndex < 0 {
		return "", errors.New("saved user message for this Dot is missing")
	}
	objective := dot.Objective
	message := conversation.Messages[messageIndex].Content
	suffix := "\n\nNewest user message:\n" + message
	prefix, ok := strings.CutSuffix(objective, suffix)
	if !ok {
		return "", errors.New("saved Dot prompt does not match its newest user message")
	}
	const contextPrefix = "\n\nComplete saved conversation context:\n"
	contextBudget := task.MaxConversationObjectiveBytes - len(objective) - len(contextPrefix)
	if contextBudget < 0 {
		return "", errors.New("saved Dot prompt exceeds the local prompt safety limit")
	}
	context, err := allConversationContext(conversation.Messages[:messageIndex], contextBudget)
	if err != nil {
		return "", fmt.Errorf("saved conversation is larger than the %d MiB local prompt safety limit", task.MaxConversationObjectiveBytes/(1024*1024))
	}
	if context != "" {
		objective = prefix + contextPrefix + context + suffix
	}
	return objective, nil
}

func allConversationContext(messages []task.ConversationMessage, maxBytes int) (string, error) {
	var context strings.Builder
	for _, message := range messages {
		label := ""
		switch message.Role {
		case "user":
			label = "User request"
		case "assistant":
			label = "Assistant reply"
		case "handoff":
			label = "Specialist response"
		default:
			continue
		}
		entry := label + ":\n" + message.Content
		separator := ""
		if context.Len() > 0 {
			separator = "\n\n"
		}
		if context.Len()+len(separator)+len(entry) > maxBytes {
			return "", errors.New("conversation context exceeds the prompt safety limit")
		}
		context.WriteString(separator)
		context.WriteString(entry)
	}
	return context.String(), nil
}

func boundedConversationContext(messages []task.ConversationMessage) string {
	recent := make([]task.ConversationMessage, 0, task.MaxConversationContextMessages)
	for i := len(messages) - 1; i >= 0 && len(recent) < task.MaxConversationContextMessages; i-- {
		if messages[i].Role == "user" || messages[i].Role == "assistant" || messages[i].Role == "handoff" {
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
	} else if message.Role == "handoff" {
		label = "Specialist response"
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
