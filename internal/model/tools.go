package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxToolOutputBytes = 8 * 1024

// Tool is one local function exposed to a model adapter. Each provider maps
// this provider-neutral contract to its own function-call wire format.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Handler     func(context.Context, string) (string, error)
}

// WithTools adds request-scoped capabilities without changing persisted chat
// data or global provider state. Built-in tools remain available unless the
// selected model is known not to support functions.
func WithTools(provider Provider, tools ...Tool) Provider {
	switch current := provider.(type) {
	case Generator:
		current.Config.Tools = append(current.Config.Tools, tools...)
		return current
	case CompatibleGenerator:
		current.Config.Tools = append(current.Config.Tools, tools...)
		return current
	case AnthropicGenerator:
		current.Config.Tools = append(current.Config.Tools, tools...)
		return current
	default:
		return provider
	}
}

// WithoutNativeTools suppresses AETHER's built-in and request-scoped functions.
func WithoutNativeTools(provider Provider) Provider {
	switch current := provider.(type) {
	case Generator:
		current.Config.DisableNativeTools = true
		return current
	case CompatibleGenerator:
		current.Config.DisableNativeTools = true
		return current
	case AnthropicGenerator:
		current.Config.DisableNativeTools = true
		return current
	default:
		return provider
	}
}

// WithMaxOutputTokens sets a one-off output cap for a provider request.
func WithMaxOutputTokens(provider Provider, limit int64) Provider {
	switch current := provider.(type) {
	case Generator:
		current.Config.MaxOutputTokens = limit
		return current
	case CompatibleGenerator:
		current.Config.MaxOutputTokens = limit
		return current
	case AnthropicGenerator:
		current.Config.MaxOutputTokens = limit
		return current
	default:
		return provider
	}
}

func registeredTools(config Config) []Tool {
	if config.DisableNativeTools {
		return nil
	}
	tools := builtinTools()
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
	}
	for _, tool := range config.Tools {
		if !validTool(tool) || seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		tools = append(tools, tool)
	}
	return tools
}

func validTool(tool Tool) bool {
	if tool.Handler == nil || strings.TrimSpace(tool.Description) == "" || len(tool.Description) > 1024 {
		return false
	}
	if len(tool.Name) == 0 || len(tool.Name) > 64 {
		return false
	}
	for _, r := range tool.Name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return tool.Parameters != nil
}

func builtinTools() []Tool {
	return []Tool{
		{
			Name:        "aether_current_time",
			Description: "Return the current local date and time. This function does not access the network or user data.",
			Parameters: map[string]any{
				"type": "object", "properties": map[string]any{}, "additionalProperties": false,
			},
			Handler: func(_ context.Context, arguments string) (string, error) {
				return executeNativeTool("aether_current_time", arguments)
			},
		},
		{
			Name:        "aether_calculate",
			Description: "Evaluate a basic arithmetic expression using numbers, parentheses, and +, -, *, /, and ^.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{"type": "string", "description": "A basic arithmetic expression, up to 256 characters."},
				},
				"required": []string{"expression"}, "additionalProperties": false,
			},
			Handler: func(_ context.Context, arguments string) (string, error) {
				return executeNativeTool("aether_calculate", arguments)
			},
		},
	}
}

func executeRegisteredTool(ctx context.Context, tools []Tool, name, arguments string) (string, error) {
	if len(arguments) > 12*1024 {
		return "", errors.New("function arguments exceed the 12 KiB limit")
	}
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		result, err := tool.Handler(ctx, arguments)
		if err != nil {
			return "", err
		}
		if !utf8.ValidString(result) {
			return "", errors.New("function result is not valid UTF-8 text")
		}
		if len(result) > maxToolOutputBytes {
			result = truncateUTF8(result, maxToolOutputBytes)
		}
		return result, nil
	}
	return "", fmt.Errorf("requested function %q is not allow-listed", name)
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
