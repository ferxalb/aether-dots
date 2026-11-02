package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

type specialistRequest struct {
	DotID          string `json:"dot_id"`
	Task           string `json:"task"`
	Constraints    string `json:"constraints,omitempty"`
	ExpectedOutput string `json:"expected_output"`
}

// WithSpecialistDelegation adds one-hop, per-Dot granted handoffs to an
// otherwise provider-neutral generator. A delegated run receives the base
// generator, so specialists cannot recursively delegate during this release.
func WithSpecialistDelegation(db *store.Store, parentDotID string, provider model.Provider, onProgress func()) model.Provider {
	tool, ok := specialistHandoffTool(db, parentDotID, provider, onProgress)
	if !ok {
		return provider
	}
	return model.WithTools(provider, tool)
}

func specialistHandoffTool(db *store.Store, parentDotID string, provider model.Provider, onProgress func()) (model.Tool, bool) {
	parentConversation, peers, ok := db.DelegationTargets(parentDotID)
	if !ok || len(parentConversation.DelegatedDotIDs) == 0 {
		return model.Tool{}, false
	}
	allowed := make(map[string]bool, len(peers))
	ids := make([]string, 0, len(peers))
	var choices strings.Builder
	for _, peer := range peers {
		allowed[peer.ID] = true
		ids = append(ids, peer.ID)
		if choices.Len() > 0 {
			choices.WriteString("; ")
		}
		choices.WriteString(peer.ID)
		choices.WriteString("=")
		choices.WriteString(shortUTF8(peer.Name, 24))
		if peer.Purpose != "" {
			choices.WriteString(" — ")
			choices.WriteString(shortUTF8(peer.Purpose, 18))
		}
	}
	if len(ids) == 0 {
		return model.Tool{}, false
	}
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dot_id": map[string]any{
				"type": "string", "enum": ids,
				"description": "Select one granted specialist. Allowed IDs: " + choices.String(),
			},
			"task": map[string]any{
				"type": "string", "maxLength": task.MaxDelegatedTaskBytes,
				"description": "The focused outcome for the specialist. Include only context needed for this task.",
			},
			"constraints": map[string]any{
				"type": "string", "maxLength": task.MaxDelegatedConstraintBytes,
				"description": "Relevant limits, source facts, or assumptions to preserve.",
			},
			"expected_output": map[string]any{
				"type": "string", "maxLength": task.MaxDelegatedOutputBytes,
				"description": "How the specialist should format or validate its result.",
			},
		},
		"required":             []string{"dot_id", "task", "expected_output"},
		"additionalProperties": false,
	}
	tool := model.Tool{
		Name:        "aether_delegate_to_dot",
		Description: fmt.Sprintf("Ask one explicitly granted specialist in the same Space to complete a typed task (up to %d handoffs per run). Only the task, constraints, and expected output you provide are sent. The specialist uses its own profile and private conversation history; its task and result are saved in both Dot histories. No files, folders, apps, accounts, or external services are accessed.", task.MaxDelegationsPerRun),
		Parameters:  parameters,
	}
	tool.Handler = func(ctx context.Context, rawArguments string) (string, error) {
		var request specialistRequest
		if err := decodeSpecialistRequest(rawArguments, &request); err != nil {
			_ = db.AppendEvent(parentDotID, "delegation_denied", "A specialist handoff was rejected because its arguments were invalid.")
			return "", fmt.Errorf("invalid specialist handoff: %w", err)
		}
		if _, ok := allowed[request.DotID]; !ok {
			_ = db.AppendEvent(parentDotID, "delegation_denied", "A specialist handoff was rejected because its target was not in the current grant list.")
			return "", errors.New("the selected specialist is not in this Dot's grant list")
		}
		child, err := db.CreateDelegatedConversationRun(parentDotID, request.DotID, request.Task, request.Constraints, request.ExpectedOutput)
		if err != nil {
			_ = db.AppendEvent(parentDotID, "delegation_denied", "A specialist handoff was rejected by the current access policy.")
			return "", err
		}
		if onProgress != nil {
			onProgress()
		}
		if err := RunWithProgress(ctx, db, child.ID, provider, nil, onProgress); err != nil {
			_ = db.AppendEvent(parentDotID, "delegation_failed", "The granted specialist did not complete the handoff.")
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("the specialist task did not complete: %w", err)
		}
		report, ok := db.ConversationResult(child.ID)
		if !ok {
			_ = db.AppendEvent(parentDotID, "delegation_failed", "The granted specialist completed without a saved response.")
			return "", errors.New("the specialist completed without a saved response")
		}
		if err := db.AppendConversationHandoffResult(parentDotID, child.ID, report); err != nil {
			return "", err
		}
		_ = db.SetCheckpoint(parentDotID, "generating response")
		if onProgress != nil {
			onProgress()
		}
		return report, nil
	}
	return tool, true
}

func decodeSpecialistRequest(raw string, target *specialistRequest) error {
	if len(raw) > 12*1024 {
		return errors.New("arguments exceed the 12 KiB limit")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func shortUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && end < len(value) && (value[end]&0xc0) == 0x80 {
		end--
	}
	return value[:end]
}
