package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/integrations/mcp"
	"aether-dots/internal/task"
)

func (s *Store) AddMCPServer(name, endpoint string) (task.MCPServer, error) {
	name = strings.TrimSpace(name)
	endpoint, err := mcp.NormalizeEndpoint(endpoint)
	if err != nil {
		return task.MCPServer{}, err
	}
	if name == "" || len(name) > task.MaxMCPServerNameBytes || !utf8.ValidString(name) {
		return task.MCPServer{}, fmt.Errorf("MCP server name must be valid UTF-8 text of at most %d bytes", task.MaxMCPServerNameBytes)
	}
	id, err := newID()
	if err != nil {
		return task.MCPServer{}, err
	}
	server := task.MCPServer{ID: id, Name: name, Endpoint: endpoint, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	err = s.change(func(db *task.Database) error {
		if len(db.MCPServers) >= task.MaxMCPServers {
			return fmt.Errorf("AETHER supports up to %d loopback MCP servers", task.MaxMCPServers)
		}
		for _, existing := range db.MCPServers {
			if existing.Endpoint == endpoint {
				return errors.New("this loopback MCP endpoint is already connected")
			}
		}
		db.MCPServers = append(db.MCPServers, server)
		return nil
	})
	return server, err
}

// Dir returns the data directory shared by Desktop and the CLI.
func (s *Store) Dir() string { return filepath.Dir(s.path) }

// MCPConnectOptions returns the connection options for a configured server,
// attaching its saved bearer token when one exists. A token the user saved
// must be usable or the connection fails; silently dropping it would turn an
// authorization problem into a confusing outage.
func (s *Store) MCPConnectOptions(serverID string) ([]mcp.Option, error) {
	token, err := mcp.LoadToken(s.Dir(), serverID)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	return []mcp.Option{mcp.WithBearer(token)}, nil
}

// SetMCPToken stores the bearer token for a configured server outside dots.json.
func (s *Store) SetMCPToken(serverID, token string) error {
	return mcp.SaveToken(s.Dir(), serverID, token)
}

func (s *Store) RemoveMCPServer(id string) error {
	if err := s.removeMCPServer(id); err != nil {
		return err
	}
	return mcp.DeleteToken(s.Dir(), id)
}

func (s *Store) removeMCPServer(id string) error {
	return s.change(func(db *task.Database) error {
		index := -1
		for i := range db.MCPServers {
			if db.MCPServers[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("MCP server not found")
		}
		db.MCPServers = append(db.MCPServers[:index], db.MCPServers[index+1:]...)
		for i := range db.Conversations {
			conversation := &db.Conversations[i]
			remaining := conversation.MCPToolGrants[:0]
			for _, grant := range conversation.MCPToolGrants {
				if grant.ServerID != id {
					remaining = append(remaining, grant)
				}
			}
			conversation.MCPToolGrants = remaining
			if len(remaining) == 0 {
				conversation.MCPGrantSpaceID = ""
				conversation.MCPModelDisclosureAccepted = false
			}
		}
		return nil
	})
}

func (s *Store) RefreshMCPServerTools(serverID string, tools []task.MCPTool) error {
	if len(tools) > task.MaxMCPServerTools {
		return fmt.Errorf("MCP server exposes more than the %d tool limit", task.MaxMCPServerTools)
	}
	seen := make(map[string]bool, len(tools))
	for i := range tools {
		fingerprint, err := task.MCPToolFingerprint(tools[i])
		if err != nil {
			return err
		}
		if seen[tools[i].Name] {
			return fmt.Errorf("MCP server returned duplicate tool %q", tools[i].Name)
		}
		seen[tools[i].Name] = true
		if tools[i].Fingerprint != "" && tools[i].Fingerprint != fingerprint {
			return fmt.Errorf("MCP tool %q fingerprint is invalid", tools[i].Name)
		}
		tools[i].Fingerprint = fingerprint
	}
	s.mu.RLock()
	unchanged := false
	for _, server := range s.db.MCPServers {
		if server.ID == serverID {
			unchanged = sameMCPToolCatalog(server.Tools, tools)
			break
		}
	}
	s.mu.RUnlock()
	if unchanged {
		return nil
	}
	return s.change(func(db *task.Database) error {
		index := -1
		for i := range db.MCPServers {
			if db.MCPServers[i].ID == serverID {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("MCP server not found")
		}
		server := &db.MCPServers[index]
		available := make(map[string]string, len(tools))
		for _, tool := range tools {
			available[tool.Name] = tool.Fingerprint
		}
		for i := range db.Conversations {
			conversation := &db.Conversations[i]
			remaining := conversation.MCPToolGrants[:0]
			for _, grant := range conversation.MCPToolGrants {
				if grant.ServerID != serverID {
					remaining = append(remaining, grant)
					continue
				}
				if fingerprint, ok := available[grant.ToolName]; ok && fingerprint == grant.Fingerprint {
					remaining = append(remaining, grant)
				}
			}
			conversation.MCPToolGrants = remaining
			if len(remaining) == 0 {
				conversation.MCPGrantSpaceID = ""
				conversation.MCPModelDisclosureAccepted = false
			}
		}
		server.Tools = append([]task.MCPTool(nil), tools...)
		server.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// RevokeMCPServerGrants removes all saved Dot grants for a server whose live
// tool catalog could not be validated. A malformed catalog must never leave a
// dormant grant that could reactivate after the server changes back.
func (s *Store) RevokeMCPServerGrants(serverID string) error {
	return s.change(func(db *task.Database) error {
		for i := range db.Conversations {
			conversation := &db.Conversations[i]
			remaining := conversation.MCPToolGrants[:0]
			for _, grant := range conversation.MCPToolGrants {
				if grant.ServerID != serverID {
					remaining = append(remaining, grant)
				}
			}
			conversation.MCPToolGrants = remaining
			if len(remaining) == 0 {
				conversation.MCPGrantSpaceID = ""
				conversation.MCPModelDisclosureAccepted = false
			}
		}
		return nil
	})
}

// ClearWorkspaceMCPDisclosures invalidates consent for Dots that follow the
// workspace provider after the user switches that provider. Per-Dot provider
// overrides keep their own existing disclosure.
func (s *Store) ClearWorkspaceMCPDisclosures() error {
	s.mu.RLock()
	needed := false
	for _, conversation := range s.db.Conversations {
		if conversation.ModelProvider == "" && len(conversation.MCPToolGrants) > 0 && conversation.MCPModelDisclosureAccepted {
			needed = true
			break
		}
	}
	s.mu.RUnlock()
	if !needed {
		return nil
	}
	return s.change(func(db *task.Database) error {
		for i := range db.Conversations {
			conversation := &db.Conversations[i]
			if conversation.ModelProvider == "" && len(conversation.MCPToolGrants) > 0 {
				conversation.MCPModelDisclosureAccepted = false
			}
		}
		return nil
	})
}

func sameMCPToolCatalog(current, next []task.MCPTool) bool {
	if len(current) != len(next) {
		return false
	}
	currentByName := make(map[string]string, len(current))
	for _, tool := range current {
		currentByName[tool.Name] = tool.Fingerprint
	}
	for _, tool := range next {
		if fingerprint, ok := currentByName[tool.Name]; !ok || fingerprint != tool.Fingerprint {
			return false
		}
	}
	return true
}

// MCPToolServersForDot returns only live, explicitly granted tools for an
// interactive Dot. Routine and delegated runs never inherit MCP access.
func (s *Store) MCPToolServersForDot(dotID string) []task.MCPServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dotIndex := dotIndex(&s.db, dotID)
	if dotIndex < 0 {
		return nil
	}
	dot := s.db.Dots[dotIndex]
	if dot.ConversationID == "" || dot.RoutineID != "" || dot.DelegatedByDotID != "" {
		return nil
	}
	conversationIndex := conversationIndex(&s.db, dot.ConversationID)
	if conversationIndex < 0 {
		return nil
	}
	conversation := s.db.Conversations[conversationIndex]
	if conversation.SpaceID != dot.SpaceID || conversation.MCPGrantSpaceID != dot.SpaceID || !conversation.MCPModelDisclosureAccepted {
		return nil
	}
	grants := make(map[string]map[string]string)
	for _, grant := range conversation.MCPToolGrants {
		if grants[grant.ServerID] == nil {
			grants[grant.ServerID] = make(map[string]string)
		}
		grants[grant.ServerID][grant.ToolName] = grant.Fingerprint
	}
	servers := make([]task.MCPServer, 0, len(grants))
	for _, server := range s.db.MCPServers {
		allowed := grants[server.ID]
		if len(allowed) == 0 {
			continue
		}
		allTools := server.Tools
		server.Tools = make([]task.MCPTool, 0, len(allowed))
		for _, tool := range allTools {
			if allowed[tool.Name] == tool.Fingerprint {
				tool.InputSchema = cloneJSONMap(tool.InputSchema)
				server.Tools = append(server.Tools, tool)
			}
		}
		if len(server.Tools) > 0 {
			servers = append(servers, server)
		}
	}
	return servers
}

func (s *Store) MCPToolAuthorized(dotID, serverID, toolName, fingerprint string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index := dotIndex(&s.db, dotID)
	if index < 0 {
		return false
	}
	dot := s.db.Dots[index]
	if dot.ConversationID == "" || dot.RoutineID != "" || dot.DelegatedByDotID != "" {
		return false
	}
	ci := conversationIndex(&s.db, dot.ConversationID)
	if ci < 0 {
		return false
	}
	conversation := s.db.Conversations[ci]
	if conversation.SpaceID != dot.SpaceID || conversation.MCPGrantSpaceID != dot.SpaceID || !conversation.MCPModelDisclosureAccepted {
		return false
	}
	for _, grant := range conversation.MCPToolGrants {
		if grant.ServerID == serverID && grant.ToolName == toolName && grant.Fingerprint == fingerprint {
			for _, server := range s.db.MCPServers {
				if server.ID == serverID {
					for _, tool := range server.Tools {
						if tool.Name == toolName && tool.Fingerprint == fingerprint {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func validateMCPToolGrants(db *task.Database, requested []task.MCPToolGrant) ([]task.MCPToolGrant, error) {
	if len(requested) > task.MaxMCPDotToolGrants {
		return nil, fmt.Errorf("a Dot can be granted at most %d MCP tools", task.MaxMCPDotToolGrants)
	}
	servers := make(map[string]task.MCPServer, len(db.MCPServers))
	for _, server := range db.MCPServers {
		servers[server.ID] = server
	}
	seen := make(map[string]bool, len(requested))
	grants := make([]task.MCPToolGrant, 0, len(requested))
	for _, grant := range requested {
		server, ok := servers[grant.ServerID]
		if !ok {
			return nil, errors.New("MCP server is no longer connected")
		}
		valid := false
		for _, tool := range server.Tools {
			if tool.Name == grant.ToolName && tool.Fingerprint == grant.Fingerprint {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("MCP tool %q changed; refresh and review its new definition before granting it", grant.ToolName)
		}
		key := grant.ServerID + "\x00" + grant.ToolName
		if seen[key] {
			return nil, errors.New("MCP tool grant list contains a duplicate")
		}
		seen[key] = true
		grants = append(grants, grant)
	}
	return grants, nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

// SetConversationMCPGrants replaces one Dot conversation's MCP tool grants.
// It applies the same validation, fingerprint, Space-binding, and model-data
// disclosure rules as the Desktop profile editor. Passing no grants revokes all.
func (s *Store) SetConversationMCPGrants(conversationID string, grants []task.MCPToolGrant, disclosureAccepted bool) error {
	return s.change(func(db *task.Database) error {
		ci := conversationIndex(db, conversationID)
		if ci < 0 {
			return errors.New("Dot not found")
		}
		validated, err := validateMCPToolGrants(db, grants)
		if err != nil {
			return err
		}
		if len(validated) > 0 && !disclosureAccepted {
			return errors.New("acknowledge that MCP tool inputs and returned data may be sent to the configured model before granting access")
		}
		conversation := &db.Conversations[ci]
		conversation.MCPToolGrants = validated
		conversation.MCPGrantSpaceID = ""
		conversation.MCPModelDisclosureAccepted = len(validated) > 0 && disclosureAccepted
		if len(validated) > 0 {
			conversation.MCPGrantSpaceID = conversation.SpaceID
		}
		conversation.UpdatedAt = time.Now().UTC()
		return nil
	})
}
