package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aether-dots/internal/task"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConvertTools validates a server's advertised tools into the bounded,
// fingerprinted catalog that Desktop and CLI both store for review and grants.
func ConvertTools(remoteTools []*mcpsdk.Tool) ([]task.MCPTool, error) {
	if len(remoteTools) > task.MaxMCPServerTools {
		return nil, fmt.Errorf("MCP server exposes more than %d tools", task.MaxMCPServerTools)
	}
	tools := make([]task.MCPTool, 0, len(remoteTools))
	for _, remote := range remoteTools {
		if remote == nil {
			return nil, errors.New("MCP server returned an empty tool definition")
		}
		schemaJSON, err := json.Marshal(remote.InputSchema)
		if err != nil || len(schemaJSON) > task.MaxMCPToolSchemaBytes {
			return nil, fmt.Errorf("MCP tool %q has an invalid or oversized input schema", remote.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(schemaJSON, &schema); err != nil || schema == nil {
			return nil, fmt.Errorf("MCP tool %q has an invalid input schema", remote.Name)
		}
		description := strings.TrimSpace(remote.Description)
		tool := task.MCPTool{Name: remote.Name, Description: description, InputSchema: schema}
		fingerprint, err := task.MCPToolFingerprint(tool)
		if err != nil {
			return nil, fmt.Errorf("MCP tool %q cannot be granted: %w", remote.Name, err)
		}
		tool.Fingerprint = fingerprint
		tools = append(tools, tool)
	}
	return tools, nil
}
