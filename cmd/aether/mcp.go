package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/engine"
	mcpclient "aether-dots/internal/integrations/mcp"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

const cliVersion = "0.1.0"

// Default Midnight tool sets. Memory tools are granted by default; code
// retrieval and destructive tools are separate explicit opt-ins.
var (
	midnightMemoryTools = []string{"recall", "remember", "list_memories"}
	midnightCodeTools   = []string{"search_code", "open_range", "repo_map", "index_status"}
)

const scopeMarker = "Midnight memory scope:"

func dotScope(conversationID string) string { return "dot:" + conversationID }

func mcpCommand(db *store.Store, args []string) error {
	usage := errors.New("usage: aether mcp [list|add <name> <endpoint>|refresh <server-id>|remove <server-id>|grant <dot-id> <server-id> <tool>...|revoke <dot-id>]")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "list":
		snapshot := db.Snapshot()
		for _, server := range snapshot.MCPServers {
			names := make([]string, 0, len(server.Tools))
			for _, tool := range server.Tools {
				names = append(names, tool.Name)
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", server.ID, server.Name, server.Endpoint, strings.Join(names, ","))
		}
		return nil
	case "add":
		if len(args) != 3 {
			return usage
		}
		server, err := addAndDiscover(db, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("Connected %s (%s) with %d tools\n", server.Name, server.ID, len(server.Tools))
		return nil
	case "refresh":
		if len(args) != 2 {
			return usage
		}
		server, err := refreshServer(db, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Refreshed %s: %d tools\n", server.Name, len(server.Tools))
		return nil
	case "remove":
		if len(args) != 2 {
			return usage
		}
		return db.RemoveMCPServer(args[1])
	case "grant":
		fs := flag.NewFlagSet("mcp grant", flag.ContinueOnError)
		accept := fs.Bool("accept-model-disclosure", false, "acknowledge tool inputs/outputs may be sent to the Dot's model provider")
		positional, flags := splitFlags(args[1:])
		if err := fs.Parse(flags); err != nil {
			return err
		}
		if len(positional) < 3 {
			return usage
		}
		return grantTools(db, positional[0], positional[1], positional[2:], *accept)
	case "revoke":
		if len(args) != 2 {
			return usage
		}
		return db.SetConversationMCPGrants(args[1], nil, false)
	default:
		return usage
	}
}

// splitFlags separates positional arguments from -flag arguments so flags may
// follow positionals (the standard flag package stops at the first positional).
func splitFlags(args []string) (positional, flags []string) {
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
			if !strings.Contains(args[i], "=") && i+1 < len(args) && !isBoolFlag(args[i]) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, args[i])
	}
	return
}

func isBoolFlag(name string) bool {
	switch strings.TrimLeft(name, "-") {
	case "accept-model-disclosure", "code", "forget", "json", "derived":
		return true
	}
	return false
}

func discoverTools(ctx context.Context, endpoint string) ([]task.MCPTool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session, err := mcpclient.Connect(ctx, cliVersion, endpoint)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	remote, err := session.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	return mcpclient.ConvertTools(remote)
}

func addAndDiscover(db *store.Store, name, endpoint string) (task.MCPServer, error) {
	server, err := db.AddMCPServer(name, endpoint)
	if err != nil {
		return server, err
	}
	return refreshServer(db, server.ID)
}

func findServer(db *store.Store, id string) (task.MCPServer, bool) {
	for _, server := range db.Snapshot().MCPServers {
		if server.ID == id {
			return server, true
		}
	}
	return task.MCPServer{}, false
}

func refreshServer(db *store.Store, id string) (task.MCPServer, error) {
	server, ok := findServer(db, id)
	if !ok {
		return server, errors.New("MCP server not found")
	}
	tools, err := discoverTools(context.Background(), server.Endpoint)
	if err != nil {
		return server, fmt.Errorf("could not refresh %s: %w", server.Name, err)
	}
	if err := db.RefreshMCPServerTools(id, tools); err != nil {
		return server, err
	}
	server, _ = findServer(db, id)
	return server, nil
}

func grantTools(db *store.Store, dotID, serverID string, names []string, accept bool) error {
	server, ok := findServer(db, serverID)
	if !ok {
		return errors.New("MCP server not found")
	}
	conversation, ok := conversationFor(db, dotID)
	if !ok {
		return errors.New("Dot not found")
	}
	grants := append([]task.MCPToolGrant(nil), conversation.MCPToolGrants...)
	for _, name := range names {
		var found *task.MCPTool
		for i := range server.Tools {
			if server.Tools[i].Name == name {
				found = &server.Tools[i]
			}
		}
		if found == nil {
			return fmt.Errorf("server %s does not advertise tool %q (run `aether mcp refresh %s`)", server.Name, name, server.ID)
		}
		replaced := false
		for i := range grants {
			if grants[i].ServerID == server.ID && grants[i].ToolName == name {
				grants[i].Fingerprint, replaced = found.Fingerprint, true
			}
		}
		if !replaced {
			grants = append(grants, task.MCPToolGrant{ServerID: server.ID, ToolName: name, Fingerprint: found.Fingerprint})
		}
	}
	accept = accept || conversation.MCPModelDisclosureAccepted
	return db.SetConversationMCPGrants(dotID, grants, accept)
}

func conversationFor(db *store.Store, id string) (task.Conversation, bool) {
	for _, conversation := range db.Snapshot().Conversations {
		if conversation.ID == id {
			return conversation, true
		}
	}
	return task.Conversation{}, false
}

// midnightCommand wires a Dot to a user-started loopback Midnight server.
func midnightCommand(db *store.Store, args []string) error {
	usage := errors.New("usage: aether midnight enable <dot-id> --endpoint http://127.0.0.1:<port>/mcp --accept-model-disclosure [--code] [--forget]")
	if len(args) < 2 || args[0] != "enable" {
		return usage
	}
	positional, flags := splitFlags(args[1:])
	fs := flag.NewFlagSet("midnight enable", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "loopback Midnight MCP endpoint")
	accept := fs.Bool("accept-model-disclosure", false, "acknowledge memories and code snippets may be sent to the Dot's model provider")
	code := fs.Bool("code", false, "also grant read-only code retrieval tools")
	forget := fs.Bool("forget", false, "also grant the forget tool")
	if err := fs.Parse(flags); err != nil || len(positional) != 1 || *endpoint == "" {
		return usage
	}
	conversation, ok := conversationFor(db, positional[0])
	if !ok {
		return errors.New("Dot not found")
	}
	normalized, err := mcpclient.NormalizeEndpoint(*endpoint)
	if err != nil {
		return err
	}
	var server task.MCPServer
	for _, existing := range db.Snapshot().MCPServers {
		if existing.Endpoint == normalized {
			server = existing
		}
	}
	if server.ID == "" {
		if server, err = db.AddMCPServer("Midnight", normalized); err != nil {
			return err
		}
	}
	if server, err = refreshServer(db, server.ID); err != nil {
		return err
	}
	tools := append([]string(nil), midnightMemoryTools...)
	if *code {
		tools = append(tools, midnightCodeTools...)
	}
	if *forget {
		tools = append(tools, "forget")
	}
	if err := grantTools(db, conversation.ID, server.ID, tools, *accept); err != nil {
		return err
	}
	if !strings.Contains(conversation.Instructions, scopeMarker) {
		instructions := strings.TrimSpace(conversation.Instructions + "\n" + fmt.Sprintf(
			"%s %s. Use this exact scope when calling Midnight recall, remember, list_memories and forget. Recall before answering questions about prior facts, cite memory ids, and never claim a memory you did not retrieve.",
			scopeMarker, dotScope(conversation.ID)))
		if err := db.UpdateConversationProfile(conversation.ID, conversation.SpaceID, conversation.Name, conversation.Icon, conversation.Color, conversation.Purpose, instructions); err != nil {
			return err
		}
	}
	fmt.Printf("Dot %s can use Midnight tools [%s] on scope %s\n", conversation.ID, strings.Join(tools, ", "), dotScope(conversation.ID))
	fmt.Printf("Run Midnight with: aether-midnight serve-http --memory-scope %s\n", dotScope(conversation.ID))
	return nil
}

// memoryCommand performs user-initiated memory operations against Midnight.
func memoryCommand(db *store.Store, args []string) error {
	usage := errors.New("usage: aether memory <remember|recall|list|forget> <dot-id> [--endpoint URL] ...\n  remember <dot-id> <text> [--derived] [--supersedes <id>] [--source-uri URI]\n  recall <dot-id> <query> [--limit N]\n  list <dot-id>\n  forget <dot-id> <memory-id>")
	if len(args) < 2 {
		return usage
	}
	positional, flags := splitFlags(args[1:])
	fs := flag.NewFlagSet("memory", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "Midnight endpoint (default: the connected server that offers recall)")
	supersedes := fs.Int64("supersedes", 0, "memory id this fact replaces")
	sourceURI := fs.String("source-uri", "", "provenance URI for the memory")
	limit := fs.Int("limit", 5, "maximum recall results")
	derived := fs.Bool("derived", false, "store a derived (supersedable) fact with provenance")
	if err := fs.Parse(flags); err != nil || len(positional) < 1 {
		return usage
	}
	conversation, ok := conversationFor(db, positional[0])
	if !ok {
		return errors.New("Dot not found")
	}
	scope := dotScope(conversation.ID)
	target := *endpoint
	if target == "" {
		for _, server := range db.Snapshot().MCPServers {
			for _, tool := range server.Tools {
				if tool.Name == "recall" {
					target = server.Endpoint
				}
			}
		}
	}
	if target == "" {
		return errors.New("no Midnight server connected; run `aether midnight enable` or pass --endpoint")
	}
	var tool string
	call := map[string]any{"scope": scope}
	switch args[0] {
	case "remember":
		if len(positional) != 2 {
			return usage
		}
		tool = "remember"
		call["kind"], call["content"] = "source", positional[1]
		if *sourceURI != "" {
			call["source_uri"] = *sourceURI
		}
		if *supersedes > 0 {
			call["supersedes_id"] = *supersedes
		}
		if *derived || *supersedes > 0 {
			// Midnight requires superseding facts to be derived, dated, and cited.
			call["kind"] = "derived"
			call["valid_from"] = time.Now().UTC().Format(time.RFC3339Nano)
			if *sourceURI == "" {
				call["source_uri"] = "aether-cli://" + conversation.ID
			}
		}
	case "recall":
		if len(positional) != 2 {
			return usage
		}
		tool = "recall"
		call["query"], call["limit"] = positional[1], *limit
	case "list":
		tool = "list_memories"
	case "forget":
		if len(positional) != 2 {
			return usage
		}
		var id int64
		if _, err := fmt.Sscan(positional[1], &id); err != nil || id < 1 {
			return errors.New("memory id must be a positive integer")
		}
		tool, call["id"] = "forget", id
	default:
		return usage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := mcpclient.Connect(ctx, cliVersion, target)
	if err != nil {
		return err
	}
	defer session.Close()
	result, err := session.CallTool(ctx, tool, call)
	if err != nil {
		return err
	}
	var text strings.Builder
	for _, content := range result.Content {
		if t, ok := content.(interface{ MarshalJSON() ([]byte, error) }); ok {
			raw, _ := t.MarshalJSON()
			var c struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(raw, &c)
			text.WriteString(c.Text)
		}
	}
	if result.IsError {
		return errors.New(text.String())
	}
	fmt.Println(text.String())
	return nil
}

// doctorCommand reports local state and probes each connected Midnight server.
func doctorCommand(db *store.Store, dataDir string) error {
	snapshot := db.Snapshot()
	running, paused, waiting := 0, 0, 0
	for _, dot := range snapshot.Dots {
		switch dot.Status {
		case task.Running:
			running++
		case task.Paused:
			paused++
		case task.WaitingInput:
			waiting++
		}
	}
	fmt.Printf("data dir:    %s\nschema:      %d\nspaces:      %d\ndots:        %d\nmessages:    running=%d paused=%d waiting=%d\n",
		dataDir, snapshot.SchemaVersion, len(snapshot.Spaces), len(snapshot.Conversations), running, paused, waiting)
	provider := model.NewProvider(model.FromEnvironment())
	ready, reason := provider.Readiness()
	fmt.Printf("model:       %s ready=%v %s\n", provider.Description(), ready, reason)
	problems := 0
	for _, server := range snapshot.MCPServers {
		start := time.Now()
		tools, err := discoverTools(context.Background(), server.Endpoint)
		if err != nil {
			problems++
			fmt.Printf("mcp %-10s UNREACHABLE %s (%v)\n", server.Name, server.Endpoint, err)
			continue
		}
		stale := len(tools) != len(server.Tools)
		for i := range tools {
			for _, cached := range server.Tools {
				if cached.Name == tools[i].Name && cached.Fingerprint != tools[i].Fingerprint {
					stale = true
				}
			}
		}
		fmt.Printf("mcp %-10s ok %d tools in %s%s\n", server.Name, len(tools), time.Since(start).Round(time.Millisecond),
			map[bool]string{true: " (catalog changed: run `aether mcp refresh " + server.ID + "`)", false: ""}[stale])
	}
	if problems > 0 {
		return fmt.Errorf("%d MCP server(s) unreachable", problems)
	}
	return nil
}

func interactiveGenerator(ctx context.Context, db *store.Store, id string) model.Provider {
	service := newGitHubService()
	providerForDot := func(dotID string) model.Provider {
		provider := engine.WithGitHubTools(db, dotID, model.NewProvider(model.FromEnvironment()), service)
		return engine.WithMCPTools(ctx, db, dotID, provider)
	}
	return engine.WithSpecialistDelegationFactory(db, id, providerForDot(id), func(delegateID string) model.Provider {
		// Delegated Dots never receive MCP tools.
		return engine.WithGitHubTools(db, delegateID, model.NewProvider(model.FromEnvironment()), service)
	}, nil)
}
