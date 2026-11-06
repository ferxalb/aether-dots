package store

import (
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
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions, delegatedDotIDs, nil, nil, nil, nil, nil, nil, nil, nil)
}

// SetConversationContextSummary saves user-authored or user-approved memory
// without changing or removing the full local transcript.
func (s *Store) SetConversationContextSummary(id, summary string) error {
	summary = strings.TrimSpace(summary)
	if len(summary) > task.MaxConversationSummaryBytes || !utf8.ValidString(summary) {
		return fmt.Errorf("conversation summary must be valid UTF-8 text of at most %d bytes", task.MaxConversationSummaryBytes)
	}
	return s.change(func(db *task.Database) error {
		i := conversationIndex(db, id)
		if i < 0 {
			return errors.New("conversation not found")
		}
		if summary != db.Conversations[i].ContextSummary {
			db.Conversations[i].ContextSummaryThroughMessageID = ""
		}
		db.Conversations[i].ContextSummary = summary
		db.Conversations[i].UpdatedAt = time.Now().UTC()
		return nil
	})
}

// SetConversationMemory commits a generated summary only if the transcript has
// not changed since the compaction input was captured.
func (s *Store) SetConversationMemory(id, summary, throughMessageID, expectedLastMessageID string) error {
	summary = strings.TrimSpace(summary)
	if len(summary) > task.MaxConversationSummaryBytes || !utf8.ValidString(summary) {
		return fmt.Errorf("conversation summary must be valid UTF-8 text of at most %d bytes", task.MaxConversationSummaryBytes)
	}
	return s.change(func(db *task.Database) error {
		i := conversationIndex(db, id)
		if i < 0 {
			return errors.New("conversation not found")
		}
		conversation := &db.Conversations[i]
		latest := ""
		if len(conversation.Messages) > 0 {
			latest = conversation.Messages[len(conversation.Messages)-1].ID
		}
		if latest != expectedLastMessageID {
			return errors.New("the conversation changed while memory was being compressed; retry the update")
		}
		if throughMessageID != expectedLastMessageID {
			return errors.New("compressed memory must cover the captured transcript")
		}
		conversation.ContextSummary = summary
		conversation.ContextSummaryThroughMessageID = throughMessageID
		conversation.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// UpdateConversationProfileAndGitHub saves the Dot profile and its independent
// repository grants in one transaction.
func (s *Store) UpdateConversationProfileAndGitHub(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs []string, githubRepositoryIDs []int64, disclosureAccepted bool) error {
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions, [][]string{delegatedDotIDs}, &githubRepositoryIDs, &disclosureAccepted, nil, nil, nil, nil, nil, nil)
}

// UpdateConversationProfileAndIntegrations saves a Dot profile and its
// independent GitHub and Computer Use grants in one transaction.
func (s *Store) UpdateConversationProfileAndIntegrations(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs []string, githubRepositoryIDs []int64, githubDisclosureAccepted bool, computerUseScopes []string, computerUseDisclosureAccepted bool) error {
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions,
		[][]string{delegatedDotIDs}, &githubRepositoryIDs, &githubDisclosureAccepted,
		&computerUseScopes, &computerUseDisclosureAccepted, nil, nil, nil, nil)
}

func (s *Store) UpdateConversationProfileAndAllIntegrations(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs []string, githubRepositoryIDs []int64, githubDisclosureAccepted bool, computerUseScopes []string, computerUseDisclosureAccepted bool, modelProvider, modelName string, mcpToolGrants []task.MCPToolGrant, mcpDisclosureAccepted bool) error {
	return s.updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions,
		[][]string{delegatedDotIDs}, &githubRepositoryIDs, &githubDisclosureAccepted,
		&computerUseScopes, &computerUseDisclosureAccepted,
		&modelProvider, &modelName, &mcpToolGrants, &mcpDisclosureAccepted)
}

func (s *Store) updateConversationProfile(id, spaceID, name, icon, color, purpose, instructions string, delegatedDotIDs [][]string, githubRepositoryIDs *[]int64, disclosureAccepted *bool, computerUseScopes *[]string, computerUseDisclosureAccepted *bool, modelProvider, modelName *string, mcpToolGrants *[]task.MCPToolGrant, mcpDisclosureAccepted *bool) error {
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
	if modelProvider != nil && modelName != nil {
		provider := strings.TrimSpace(*modelProvider)
		modelNameValue := strings.TrimSpace(*modelName)
		if provider == "" && modelNameValue != "" {
			return errors.New("a Dot-specific model name requires a provider")
		}
		if provider != "" && !oneOf(provider, "openai", "anthropic", "xai", "openrouter", "openai-compatible") {
			return errors.New("Dot model provider is not supported")
		}
		if len(modelNameValue) > 256 || (provider != "" && modelNameValue == "") {
			return errors.New("Dot model name must contain 1–256 bytes when a provider override is selected")
		}
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
		delegateGrants, err := validateDelegates(db, id, spaceID, requestedDelegates)
		if err != nil {
			return err
		}
		if previousSpaceID != spaceID && githubRepositoryIDs != nil && len(*githubRepositoryIDs) > 0 {
			return errors.New("save the Dot's new Space before granting GitHub repositories again")
		}
		if previousSpaceID != spaceID && computerUseScopes != nil && len(*computerUseScopes) > 0 {
			return errors.New("save the Dot's new Space before granting Computer Use again")
		}
		if previousSpaceID != spaceID && mcpToolGrants != nil && len(*mcpToolGrants) > 0 {
			return errors.New("save the Dot's new Space before granting MCP tools again")
		}
		if githubRepositoryIDs != nil {
			githubGrants, err := validateGitHubRepositoryGrants(db, *githubRepositoryIDs)
			if err != nil {
				return err
			}
			if len(githubGrants) > 0 && (disclosureAccepted == nil || !*disclosureAccepted) {
				return errors.New("acknowledge that selected GitHub data may be sent to the configured model before granting access")
			}
			conversation.GitHubRepositoryIDs = githubGrants
			conversation.GitHubGrantSpaceID = ""
			conversation.GitHubModelDisclosureAccepted = len(githubGrants) > 0 && *disclosureAccepted
			if len(githubGrants) > 0 {
				conversation.GitHubGrantSpaceID = spaceID
			}
		} else if previousSpaceID != spaceID {
			conversation.GitHubRepositoryIDs = nil
			conversation.GitHubGrantSpaceID = ""
			conversation.GitHubModelDisclosureAccepted = false
			disableConversationGitHubRoutines(db, conversation.ID)
		}
		if computerUseScopes != nil {
			grants, err := validateComputerUseScopes(*computerUseScopes)
			if err != nil {
				return err
			}
			accepted := computerUseDisclosureAccepted != nil && *computerUseDisclosureAccepted
			if len(grants) > 0 && !accepted {
				return errors.New("acknowledge that Computer Use observations may be sent to the configured model before granting access")
			}
			conversation.ComputerUseScopes = grants
			conversation.ComputerUseGrantSpaceID = ""
			conversation.ComputerUseModelDisclosureAccepted = len(grants) > 0 && accepted
			if len(grants) > 0 {
				conversation.ComputerUseGrantSpaceID = spaceID
			}
		} else if previousSpaceID != spaceID {
			conversation.ComputerUseScopes = nil
			conversation.ComputerUseGrantSpaceID = ""
			conversation.ComputerUseModelDisclosureAccepted = false
		}
		if mcpToolGrants != nil {
			validated, err := validateMCPToolGrants(db, *mcpToolGrants)
			if err != nil {
				return err
			}
			accepted := mcpDisclosureAccepted != nil && *mcpDisclosureAccepted
			if len(validated) > 0 && !accepted {
				return errors.New("acknowledge that MCP tool inputs and returned data may be sent to the configured model before granting access")
			}
			conversation.MCPToolGrants = validated
			conversation.MCPGrantSpaceID = ""
			conversation.MCPModelDisclosureAccepted = len(validated) > 0 && accepted
			if len(validated) > 0 {
				conversation.MCPGrantSpaceID = spaceID
			}
		} else if previousSpaceID != spaceID {
			conversation.MCPToolGrants = nil
			conversation.MCPGrantSpaceID = ""
			conversation.MCPModelDisclosureAccepted = false
		}
		if modelProvider != nil && modelName != nil {
			conversation.ModelProvider = strings.TrimSpace(*modelProvider)
			conversation.Model = strings.TrimSpace(*modelName)
		}
		conversation.DelegatedDotIDs = delegateGrants
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
		if existing.ConversationID == conversationID {
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

// RecoverInterruptedRuns pauses conversation Dots whose owning process is gone
// (see runlease.go). Dots running in any live Desktop or CLI process keep their
// state, so starting Desktop never interrupts an active CLI run.
func (s *Store) RecoverInterruptedRuns() (int, error) {
	recovered, _, err := s.RecoverOrphanedRuns()
	return recovered, err
}

func conversationIndex(db *task.Database, id string) int {
	for i := range db.Conversations {
		if db.Conversations[i].ID == id {
			return i
		}
	}
	return -1
}

// ConversationModel returns only the optional per-Dot model override. Provider
// secrets and full conversation history remain outside this lookup.
func (s *Store) ConversationModel(dotID string) (provider, modelName string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	di := dotIndex(&s.db, dotID)
	if di < 0 || s.db.Dots[di].ConversationID == "" {
		return "", "", false
	}
	ci := conversationIndex(&s.db, s.db.Dots[di].ConversationID)
	if ci < 0 {
		return "", "", false
	}
	conversation := s.db.Conversations[ci]
	return conversation.ModelProvider, conversation.Model, true
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

// ConversationObjective assembles a compact provider prompt from the durable
// profile, optional user-maintained memory, shared Space context, and recent
// messages. The complete transcript stays saved locally.
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
	var spaceContext string
	for i := range s.db.Spaces {
		if s.db.Spaces[i].ID == conversation.SpaceID {
			spaceContext = s.db.Spaces[i].Context
			break
		}
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
	const (
		spacePrefix  = "\n\nShared Space context:\n"
		memoryPrefix = "\n\nSaved conversation summary:\n"
		recentPrefix = "\n\nRecent saved conversation context:\n"
	)
	if len(objective) > task.MaxConversationObjectiveBytes {
		return "", errors.New("saved Dot prompt exceeds the local prompt safety limit")
	}
	var context strings.Builder
	if strings.TrimSpace(spaceContext) != "" {
		context.WriteString(spacePrefix)
		context.WriteString(strings.TrimSpace(spaceContext))
	}
	if strings.TrimSpace(conversation.ContextSummary) != "" {
		context.WriteString(memoryPrefix)
		context.WriteString(strings.TrimSpace(conversation.ContextSummary))
	}
	contextStart := 0
	if throughID := conversation.ContextSummaryThroughMessageID; throughID != "" {
		for i := range conversation.Messages {
			if i < messageIndex && conversation.Messages[i].ID == throughID {
				contextStart = i + 1
				break
			}
		}
	}
	recent := boundedConversationContext(conversation.Messages[contextStart:messageIndex])
	if recent != "" {
		context.WriteString(recentPrefix)
		context.WriteString(recent)
	}
	if context.Len()+len(prefix)+len(suffix) > task.MaxConversationObjectiveBytes {
		return "", fmt.Errorf("saved conversation context exceeds the %d MiB local prompt safety limit", task.MaxConversationObjectiveBytes/(1024*1024))
	}
	objective = prefix + context.String() + suffix
	return objective, nil
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
