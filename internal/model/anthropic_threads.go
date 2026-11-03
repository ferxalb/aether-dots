package model

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

const (
	newestUserMessageMarker = "\n\nNewest user message:\n"
	recentContextMarker     = "\n\nRecent saved conversation context:\n"
	maxAnthropicThreadTurns = 32
	maxAnthropicThreadMsgs  = 96
	maxAnthropicThreadCache = 32
	maxAnthropicThreadBytes = 8 * 1024 * 1024
)

var anthropicThreadIdleLimit = 50 * time.Minute

var anthropicThreadCache = struct {
	sync.Mutex
	entries map[string]*anthropicThreadEntry
}{entries: make(map[string]*anthropicThreadEntry)}

type anthropicThreadEntry struct {
	mu          sync.Mutex
	thread      *anthropic.BetaMessageThread
	fingerprint [32]byte
	lastUsed    time.Time
	turns       int
	approxBytes int
}

// ClearAnthropicThreads drops this process's references to opt-in server-side
// threads. Anthropic expires already-sent thread data according to its own TTL.
func ClearAnthropicThreads() {
	threadCache := &anthropicThreadCache
	threadCache.Lock()
	threadCache.entries = make(map[string]*anthropicThreadEntry)
	threadCache.Unlock()
}

func (g AnthropicGenerator) generateThreaded(ctx context.Context, config Config, objective string, onDelta func(string)) (response string, returnErr error) {
	prefix, newestMessage, ok := splitThreadObjective(objective)
	if !ok || strings.TrimSpace(config.PromptCacheKey) == "" {
		return g.generateStandard(ctx, config, objective, onDelta)
	}

	toolset := registeredTools(config)
	client := g.client(config)
	fingerprint := anthropicThreadFingerprint(config, prefix, toolset)
	key := anthropicThreadKey(config)
	entry := acquireAnthropicThreadEntry(key)
	entry.mu.Lock()
	defer func() {
		if returnErr != nil {
			entry.thread = nil
			entry.turns = 0
			entry.approxBytes = 0
		}
		entry.lastUsed = time.Now()
		entry.mu.Unlock()
	}()

	now := time.Now()
	if entry.thread != nil && (entry.fingerprint != fingerprint || now.Sub(entry.lastUsed) >= anthropicThreadIdleLimit || entry.turns >= maxAnthropicThreadTurns || len(entry.thread.Messages()) >= maxAnthropicThreadMsgs || entry.approxBytes+len(newestMessage) >= maxAnthropicThreadBytes) {
		entry.thread = nil
		entry.approxBytes = 0
	}
	newThread := entry.thread == nil
	if newThread {
		entry.thread = client.Beta.Messages.NewThread(anthropic.BetaMessageThreadCreateParams{
			Model:                      anthropic.Model(strings.TrimSpace(config.Model)),
			MaxTokens:                  anthropicOutputLimit(config.MaxOutputTokens),
			System:                     []anthropic.BetaTextBlockParam{{Text: anthropicInstructions(len(toolset) == 0 && len(config.Tools) > 0)}},
			Thinking:                   anthropicBetaThinkingConfig(config.Model),
			OutputConfig:               anthropicBetaOutputConfig(config),
			Tools:                      betaAnthropicToolDefinitions(toolset),
			UnsupportedRequestBehavior: anthropic.BetaMessageThreadUnsupportedRequestBehaviorContinueUnthreaded,
		})
		entry.fingerprint = fingerprint
		entry.turns = 0
		entry.approxBytes = len(objective)
	} else {
		entry.approxBytes += len(newestMessage)
	}

	userContent := newestMessage
	if newThread {
		userContent = objective
	}
	var result strings.Builder
	redactor := secretRedactor{secret: strings.TrimSpace(config.APIKey)}
	toolCallsUsed := 0
	pending := anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(userContent))
	for round := 0; round < maxToolRounds; round++ {
		calls, err := streamAnthropicThreadTurn(ctx, entry.thread, pending, config, &result, &redactor, onDelta)
		if err != nil {
			return "", err
		}
		if len(calls) == 0 {
			response = strings.TrimSpace(redactProviderText(result.String(), config.APIKey))
			if response == "" {
				return "", errors.New("Anthropic returned an empty response")
			}
			if onDelta != nil {
				if safe := redactor.push("", true); safe != "" {
					onDelta(safe)
				}
			}
			entry.turns++
			entry.approxBytes += len(response)
			if entry.approxBytes >= maxAnthropicThreadBytes {
				entry.thread = nil
			}
			return response, nil
		}
		if len(toolset) == 0 {
			return "", errors.New("the selected model returned a function call even though functions are disabled for this model")
		}
		toolCallsUsed += len(calls)
		if toolCallsUsed > maxAnthropicToolCallsPerTurn {
			return "", errors.New("the model exceeded the 8 native function call limit")
		}
		toolResults := make([]anthropic.BetaContentBlockParamUnion, 0, len(calls))
		for _, call := range calls {
			arguments := strings.TrimSpace(call.arguments.String())
			if arguments == "" {
				arguments = "{}"
			}
			output, err := executeRegisteredTool(ctx, toolset, call.toolName, arguments)
			isError := err != nil
			if err != nil {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				output = "Function error: " + err.Error()
			}
			entry.approxBytes += len(output)
			resultBlock := anthropic.NewBetaToolResultTextBlockParam(call.toolID, redactProviderText(output, config.APIKey), isError)
			toolResults = append(toolResults, anthropic.BetaContentBlockParamUnion{OfToolResult: &resultBlock})
		}
		pending = anthropic.NewBetaUserMessage(toolResults...)
	}
	return "", errors.New("the model exceeded the 4 native function rounds limit")
}

func streamAnthropicThreadTurn(ctx context.Context, thread *anthropic.BetaMessageThread, message anthropic.BetaMessageParam, config Config, result *strings.Builder, redactor *secretRedactor, onDelta func(string)) ([]*anthropicStreamBlock, error) {
	stream := thread.SendStreaming(ctx, anthropic.BetaMessageThreadSendParams{Messages: []anthropic.BetaMessageParam{message}})
	blocks := make(map[int64]*anthropicStreamBlock)
	blockOrder := make([]int64, 0, 4)
	completed := false
	for stream.Next() {
		event := stream.Current()
		if event.Type == "message_stop" {
			completed = true
		}
		switch event.Type {
		case "content_block_start":
			started := event.AsContentBlockStart()
			block := &anthropicStreamBlock{kind: started.ContentBlock.Type}
			if block.kind == "tool_use" {
				block.toolID = started.ContentBlock.ID
				block.toolName = started.ContentBlock.Name
			}
			blocks[started.Index] = block
			blockOrder = append(blockOrder, started.Index)
		case "content_block_delta":
			started := event.AsContentBlockDelta()
			block := blocks[started.Index]
			if block == nil {
				continue
			}
			switch started.Delta.Type {
			case "text_delta":
				textDelta := started.Delta.Text
				block.text.WriteString(textDelta)
				result.WriteString(textDelta)
				if onDelta != nil {
					if safe := redactor.push(textDelta, false); safe != "" {
						onDelta(safe)
					}
				}
			case "input_json_delta":
				block.arguments.WriteString(started.Delta.PartialJSON)
				if block.arguments.Len() > maxToolArguments {
					_ = stream.Close()
					return nil, errors.New("native function arguments exceeded the 12 KiB limit")
				}
			}
		}
		if result.Len() > maxAnthropicResponseBytes {
			_ = stream.Close()
			return nil, errors.New("model response exceeded the 2 MiB limit")
		}
	}
	streamErr := stream.Err()
	_ = stream.Close()
	if streamErr != nil {
		return nil, fmt.Errorf("Anthropic Message Thread stream: %w", redactProviderError(streamErr, config.APIKey))
	}
	if !completed {
		return nil, errors.New("Anthropic Message Thread stream ended before the response completed")
	}
	sort.Slice(blockOrder, func(i, j int) bool { return blockOrder[i] < blockOrder[j] })
	var calls []*anthropicStreamBlock
	for _, index := range blockOrder {
		block := blocks[index]
		if block.kind != "tool_use" {
			continue
		}
		if block.toolID == "" || block.toolName == "" {
			return nil, errors.New("Anthropic returned a function call without an ID or name")
		}
		arguments := strings.TrimSpace(block.arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(arguments), &input); err != nil || input == nil {
			return nil, errors.New("Anthropic returned invalid function arguments")
		}
		calls = append(calls, block)
	}
	return calls, nil
}

func splitThreadObjective(objective string) (prefix, newest string, ok bool) {
	before, newest, ok := strings.Cut(objective, newestUserMessageMarker)
	if !ok {
		return "", "", false
	}
	return before, newest, true
}

func anthropicThreadKey(config Config) string {
	mac := hmac.New(sha256.New, []byte(config.APIKey))
	_, _ = mac.Write([]byte(strings.TrimSpace(config.Model) + "\x00" + strings.TrimSpace(config.PromptCacheKey)))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func anthropicThreadFingerprint(config Config, prefix string, tools []Tool) [32]byte {
	stablePrefix, _, found := strings.Cut(prefix, recentContextMarker)
	if !found {
		stablePrefix = prefix
	}
	encoded, _ := json.Marshal(struct {
		Model      string
		System     string
		Prompt     string
		MaxTokens  int64
		Effort     string
		ToolSchema []anthropic.BetaToolUnionParam
	}{
		Model:      strings.TrimSpace(config.Model),
		System:     anthropicInstructions(len(tools) == 0 && len(config.Tools) > 0),
		Prompt:     stablePrefix,
		MaxTokens:  anthropicOutputLimit(config.MaxOutputTokens),
		Effort:     strings.TrimSpace(config.ReasoningEffort),
		ToolSchema: betaAnthropicToolDefinitions(tools),
	})
	return sha256.Sum256(encoded)
}

func acquireAnthropicThreadEntry(key string) *anthropicThreadEntry {
	cache := &anthropicThreadCache
	cache.Lock()
	now := time.Now()
	for cachedKey, entry := range cache.entries {
		if !entry.mu.TryLock() {
			continue
		}
		if now.Sub(entry.lastUsed) >= anthropicThreadIdleLimit {
			delete(cache.entries, cachedKey)
		}
		entry.mu.Unlock()
	}
	if entry := cache.entries[key]; entry != nil {
		cache.Unlock()
		return entry
	}
	if len(cache.entries) >= maxAnthropicThreadCache {
		oldestKey := ""
		oldestTime := now
		for cachedKey, entry := range cache.entries {
			if !entry.mu.TryLock() {
				continue
			}
			lastUsed := entry.lastUsed
			entry.mu.Unlock()
			if oldestKey == "" || lastUsed.Before(oldestTime) {
				oldestKey, oldestTime = cachedKey, lastUsed
			}
		}
		if oldestKey != "" {
			delete(cache.entries, oldestKey)
		}
	}
	entry := &anthropicThreadEntry{lastUsed: now}
	if len(cache.entries) < maxAnthropicThreadCache {
		cache.entries[key] = entry
	}
	cache.Unlock()
	return entry
}

func betaAnthropicToolDefinitions(tools []Tool) []anthropic.BetaToolUnionParam {
	definitions := make([]anthropic.BetaToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		properties, _ := tool.Parameters["properties"]
		required := stringSlice(tool.Parameters["required"])
		extra := make(map[string]any)
		if additional, ok := tool.Parameters["additionalProperties"]; ok {
			extra["additionalProperties"] = additional
		}
		definitions = append(definitions, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:           tool.Name,
			Description:    anthropic.String(tool.Description),
			Type:           anthropic.BetaToolTypeCustom,
			AllowedCallers: []string{"direct"},
			InputSchema:    anthropic.BetaToolInputSchemaParam{Properties: properties, Required: required, ExtraFields: extra},
		}})
	}
	return definitions
}
