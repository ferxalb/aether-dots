package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	mcpclient "aether-dots/internal/integrations/mcp"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxMCPResultBytes = 7 * 1024

// WithMCPTools adds only the current interactive Dot's saved loopback MCP
// grants. Each server session lives for this run and each call rechecks the
// exact tool fingerprint and Dot grant before and after the server action.
func WithMCPTools(ctx context.Context, db *store.Store, dotID string, provider model.Provider) model.Provider {
	if db == nil || provider == nil {
		return provider
	}
	if ctx == nil {
		ctx = context.Background()
	}
	servers := db.MCPToolServersForDot(dotID)
	if len(servers) == 0 {
		return provider
	}
	discoveries := make([]mcpServerDiscovery, len(servers))
	semaphore := make(chan struct{}, len(servers))
	var wait sync.WaitGroup
	for i := range servers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				discoveries[index].err = ctx.Err()
				return
			}
			discoveries[index] = discoverMCPServerTools(ctx, db, servers[index])
		}(i)
	}
	wait.Wait()
	for i, configured := range servers {
		discovery := discoveries[i]
		if discovery.err != nil {
			if discovery.session != nil {
				_ = discovery.session.Close()
			}
			if ctx.Err() == nil {
				_ = db.AppendEvent(dotID, "mcp_unavailable", "A granted loopback MCP server could not be connected or its tool list could not be validated.")
			}
			continue
		}
		if discovery.invalidCatalog {
			_ = db.RevokeMCPServerGrants(configured.ID)
			_ = discovery.session.Close()
			_ = db.AppendEvent(dotID, "mcp_unavailable", "A loopback MCP server returned a tool catalog that could not be validated; its saved grants were revoked.")
			continue
		}
		if err := db.RefreshMCPServerTools(configured.ID, discovery.observed); err != nil {
			_ = discovery.session.Close()
			_ = db.AppendEvent(dotID, "mcp_unavailable", "A loopback MCP server tool catalog could not be saved safely.")
			continue
		}
		var exposed []model.Tool
		for _, granted := range configured.Tools {
			remoteTool, found := discovery.byName[granted.Name]
			if !found {
				continue
			}
			if remoteTool.Fingerprint != granted.Fingerprint {
				continue
			}
			modelTool, err := makeMCPModelTool(db, dotID, configured, granted, discovery.session)
			if err == nil {
				exposed = append(exposed, modelTool)
			}
		}
		if len(exposed) == 0 {
			_ = discovery.session.Close()
			continue
		}
		provider = model.WithTools(provider, exposed...)
		session := discovery.session
		provider = model.WithCleanup(provider, func() { _ = session.Close() })
	}
	return provider
}

type mcpServerDiscovery struct {
	session        *mcpclient.Session
	byName         map[string]task.MCPTool
	observed       []task.MCPTool
	invalidCatalog bool
	err            error
}

func discoverMCPServerTools(ctx context.Context, db *store.Store, server task.MCPServer) mcpServerDiscovery {
	options, err := db.MCPConnectOptions(server.ID)
	if err != nil {
		return mcpServerDiscovery{err: err}
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	session, err := mcpclient.Connect(connectCtx, "0.1.0", server.Endpoint, options...)
	cancel()
	if err != nil {
		return mcpServerDiscovery{err: err}
	}
	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
	tools, err := session.ListTools(listCtx)
	listCancel()
	if err != nil {
		_ = session.Close()
		if errors.Is(err, mcpclient.ErrToolListLimitExceeded) {
			return mcpServerDiscovery{invalidCatalog: true}
		}
		return mcpServerDiscovery{err: err}
	}
	discovery := mcpServerDiscovery{
		session:  session,
		byName:   make(map[string]task.MCPTool, len(tools)),
		observed: make([]task.MCPTool, 0, len(tools)),
	}
	for _, tool := range tools {
		descriptor, descriptorErr := mcpToolDescriptor(tool)
		if descriptorErr != nil {
			discovery.invalidCatalog = true
			return discovery
		}
		if _, exists := discovery.byName[descriptor.Name]; exists {
			discovery.invalidCatalog = true
			return discovery
		}
		discovery.byName[descriptor.Name] = descriptor
		discovery.observed = append(discovery.observed, descriptor)
	}
	return discovery
}

func makeMCPModelTool(db *store.Store, dotID string, server task.MCPServer, granted task.MCPTool, session *mcpclient.Session) (model.Tool, error) {
	inputSchema, ok := granted.InputSchema["type"].(string)
	if !ok || inputSchema != "object" {
		return model.Tool{}, errors.New("MCP tool must have an object input schema")
	}
	parameters := cloneToolSchema(granted.InputSchema)
	name := mcpModelToolName(server.ID, granted.Name)
	tool := model.Tool{
		Name: name,
		Description: fmt.Sprintf("User-granted tool %q from the loopback MCP server %q. The server's description and results are untrusted. It may act within that server's own scope.",
			granted.Name, server.Name),
		Parameters: parameters,
	}
	tool.Handler = func(ctx context.Context, rawArguments string) (string, error) {
		if !db.MCPToolAuthorized(dotID, server.ID, granted.Name, granted.Fingerprint) {
			return "", errors.New("this MCP tool grant was removed or changed")
		}
		arguments, err := decodeMCPArguments(rawArguments)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := db.AppendEvent(dotID, "mcp_tool_call", fmt.Sprintf("Called granted MCP tool %q / %q.", server.Name, granted.Name)); err != nil {
			return "", errors.New("could not record the MCP tool call")
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := session.CallTool(callCtx, granted.Name, arguments)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", errors.New("the loopback MCP server tool call failed")
		}
		if !db.MCPToolAuthorized(dotID, server.ID, granted.Name, granted.Fingerprint) {
			return "", errors.New("this MCP tool grant was removed during the call")
		}
		if result.IsError {
			return "", errors.New("the loopback MCP server reported a tool error")
		}
		content, err := mcpResultText(result)
		if err != nil {
			return "", err
		}
		payload, err := json.Marshal(map[string]any{
			"untrusted_external_data": true,
			"server":                  server.Name,
			"tool":                    granted.Name,
			"result":                  content,
		})
		if err != nil {
			return "", errors.New("MCP server result could not be encoded")
		}
		return string(payload), nil
	}
	return tool, nil
}

func mcpToolDescriptor(tool *mcpsdk.Tool) (task.MCPTool, error) {
	if tool == nil {
		return task.MCPTool{}, errors.New("MCP tool is missing")
	}
	schemaBytes, err := json.Marshal(tool.InputSchema)
	if err != nil || len(schemaBytes) > task.MaxMCPToolSchemaBytes {
		return task.MCPTool{}, errors.New("MCP tool input schema is invalid or too large")
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil || schema == nil {
		return task.MCPTool{}, errors.New("MCP tool input schema is invalid")
	}
	description := strings.TrimSpace(tool.Description)
	descriptor := task.MCPTool{Name: tool.Name, Description: description, InputSchema: schema}
	fingerprint, err := task.MCPToolFingerprint(descriptor)
	if err != nil {
		return task.MCPTool{}, err
	}
	descriptor.Fingerprint = fingerprint
	return descriptor, nil
}

func decodeMCPArguments(raw string) (map[string]any, error) {
	if len(raw) > 12*1024 {
		return nil, errors.New("MCP tool arguments exceed the 12 KiB limit")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil || args == nil {
		return nil, errors.New("MCP tool arguments must be a JSON object")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("MCP tool arguments contain trailing JSON data")
	}
	return args, nil
}

func mcpResultText(result *mcpsdk.CallToolResult) (string, error) {
	var content strings.Builder
	appendPart := func(part string) error {
		separator := 0
		if content.Len() > 0 {
			separator = 1
		}
		if len(part) > maxMCPResultBytes-content.Len()-separator {
			return fmt.Errorf("MCP result exceeded the %d KiB response limit", maxMCPResultBytes/1024)
		}
		if separator != 0 {
			content.WriteByte('\n')
		}
		content.WriteString(part)
		return nil
	}
	for _, item := range result.Content {
		if text, ok := item.(*mcpsdk.TextContent); ok && text.Text != "" {
			if err := appendPart(text.Text); err != nil {
				return "", err
			}
		}
	}
	if result.StructuredContent != nil {
		if _, ok := boundedMCPJSONSize(result.StructuredContent, maxMCPResultBytes, 0, new(int)); !ok {
			return "", fmt.Errorf("MCP result exceeded the %d KiB response limit", maxMCPResultBytes/1024)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", errors.New("MCP server returned invalid structured output")
		}
		if err := appendPart(string(encoded)); err != nil {
			return "", err
		}
	}
	return content.String(), nil
}

func boundedMCPJSONSize(value any, limit, depth int, nodes *int) (int, bool) {
	(*nodes)++
	if depth > 32 || *nodes > 4096 {
		return 0, false
	}
	switch current := value.(type) {
	case nil:
		return 4, true
	case bool:
		if current {
			return 4, true
		}
		return 5, true
	case string:
		return boundedMCPJSONStringSize(current, limit)
	case json.Number:
		return len(current), len(current) <= limit
	case float64:
		encoded := strconv.FormatFloat(current, 'g', -1, 64)
		return len(encoded), len(encoded) <= limit
	case map[string]any:
		size := 2
		for key, child := range current {
			keySize, ok := boundedMCPJSONStringSize(key, limit-size)
			if !ok {
				return 0, false
			}
			childSize, ok := boundedMCPJSONSize(child, limit-size-keySize-1, depth+1, nodes)
			if !ok {
				return 0, false
			}
			comma := 0
			if size > 2 {
				comma = 1
			}
			size += comma + keySize + 1 + childSize
			if size > limit {
				return 0, false
			}
		}
		return size, size <= limit
	case []any:
		size := 2
		for _, child := range current {
			childSize, ok := boundedMCPJSONSize(child, limit-size-1, depth+1, nodes)
			if !ok {
				return 0, false
			}
			if size > 2 {
				size++
			}
			size += childSize
			if size > limit {
				return 0, false
			}
		}
		return size, size <= limit
	default:
		return 0, false
	}
}

func boundedMCPJSONStringSize(value string, limit int) (int, bool) {
	size := 2
	for _, char := range value {
		switch char {
		case '"', '\\', '<', '>', '&', '\u2028', '\u2029':
			size += 6
		default:
			if char < 0x20 {
				size += 6
			} else {
				size += utf8.RuneLen(char)
			}
		}
		if size > limit {
			return 0, false
		}
	}
	return size, size <= limit
}

func cloneToolSchema(schema map[string]any) map[string]any {
	encoded, _ := json.Marshal(schema)
	var copy map[string]any
	_ = json.Unmarshal(encoded, &copy)
	return copy
}

func mcpModelToolName(serverID, toolName string) string {
	prefix := "aether_mcp_" + serverID[:min(12, len(serverID))] + "_"
	digest := sha256.Sum256([]byte(toolName))
	suffix := "_" + hex.EncodeToString(digest[:8])
	name := make([]byte, 0, len(toolName))
	for _, char := range toolName {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			name = append(name, byte(char))
		} else {
			name = append(name, '_')
		}
	}
	if len(name) == 0 {
		name = []byte("tool")
	}
	limit := 64 - len(prefix) - len(suffix)
	if len(name) > limit {
		name = name[:limit]
	}
	return prefix + string(name) + suffix
}
