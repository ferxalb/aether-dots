package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	usage := errors.New("usage: aether mcp [list|add <name> <endpoint> [--token-file F|--token-stdin]|refresh <server-id>|remove <server-id>|grant <dot-id> <server-id> <tool>...|revoke <dot-id>]")
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
		positional, flags := splitFlags(args[1:])
		fs := flag.NewFlagSet("mcp add", flag.ContinueOnError)
		tokenFile, tokenStdin := tokenFlags(fs)
		if err := fs.Parse(flags); err != nil || len(positional) != 2 {
			return usage
		}
		token, err := readToken(*tokenFile, *tokenStdin)
		if err != nil {
			return err
		}
		server, err := addAndDiscover(db, positional[0], positional[1], token)
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

func tokenFlags(fs *flag.FlagSet) (*string, *bool) {
	return fs.String("token-file", "", "file holding the Midnight bearer token (never pass tokens as arguments)"),
		fs.Bool("token-stdin", false, "read the Midnight bearer token from standard input")
}

// readToken reads a bearer token from a file or stdin; secrets are never
// accepted as command-line arguments because argv is visible to other users.
func readToken(file string, stdin bool) (string, error) {
	switch {
	case file != "" && stdin:
		return "", errors.New("use either --token-file or --token-stdin, not both")
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read token file: %w", err)
		}
		return strings.TrimSpace(string(raw)), nil
	case stdin:
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	}
	return "", nil
}

func discoverTools(ctx context.Context, endpoint string, options ...mcpclient.Option) ([]task.MCPTool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session, err := mcpclient.Connect(ctx, cliVersion, endpoint, options...)
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

// addAndDiscover registers a server and validates it. A server that cannot be
// reached or authenticated is not left half-configured.
func addAndDiscover(db *store.Store, name, endpoint, token string) (task.MCPServer, error) {
	server, err := db.AddMCPServer(name, endpoint)
	if err != nil {
		return server, err
	}
	if token != "" {
		if err := db.SetMCPToken(server.ID, token); err != nil {
			_ = db.RemoveMCPServer(server.ID)
			return server, err
		}
	}
	refreshed, err := refreshServer(db, server.ID)
	if err != nil {
		_ = db.RemoveMCPServer(server.ID)
		return server, err
	}
	return refreshed, nil
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
	options, err := db.MCPConnectOptions(server.ID)
	if err != nil {
		return server, err
	}
	tools, err := discoverTools(context.Background(), server.Endpoint, options...)
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
	usage := errors.New("usage: aether midnight enable <dot-id> --endpoint http://127.0.0.1:<port>/mcp --token-file F|--token-stdin --accept-model-disclosure [--code] [--forget]")
	if len(args) >= 1 && args[0] == "server" {
		return midnightServerCommand(db, args[1:])
	}
	if len(args) < 2 || args[0] != "enable" {
		return usage
	}
	positional, flags := splitFlags(args[1:])
	fs := flag.NewFlagSet("midnight enable", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "loopback Midnight MCP endpoint")
	accept := fs.Bool("accept-model-disclosure", false, "acknowledge memories and code snippets may be sent to the Dot's model provider")
	code := fs.Bool("code", false, "also grant read-only code retrieval tools")
	forget := fs.Bool("forget", false, "also grant the forget tool")
	tokenFile, tokenStdin := tokenFlags(fs)
	if err := fs.Parse(flags); err != nil || len(positional) != 1 || *endpoint == "" {
		return usage
	}
	token, err := readToken(*tokenFile, *tokenStdin)
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("a Midnight token is required: run `aether-midnight token create --name dot-<id> --memory-scope dot:<id>` and pass it with --token-file or --token-stdin")
	}
	if _, err := db.SnapshotChecked(); err != nil {
		return err
	}
	conversation, ok := conversationFor(db, positional[0])
	if !ok {
		return errors.New("Dot not found")
	}
	normalized, err := mcpclient.NormalizeEndpoint(*endpoint)
	if err != nil {
		return err
	}
	// One server record per Dot: the path label keeps each Dot's token, tool
	// catalog and grants separate from every other Dot using the same Midnight.
	label := "/dot-" + conversation.ID
	if !strings.HasSuffix(normalized, label) {
		normalized += label
	}
	var server task.MCPServer
	for _, existing := range db.Snapshot().MCPServers {
		if existing.Endpoint == normalized {
			server = existing
		}
	}
	created := false
	if server.ID == "" {
		if server, err = db.AddMCPServer("Midnight dot-"+conversation.ID, normalized); err != nil {
			return err
		}
		created = true
	}
	previous, _ := mcpclient.LoadToken(db.Dir(), server.ID)
	if err = db.SetMCPToken(server.ID, token); err == nil {
		server, err = refreshServer(db, server.ID)
	}
	if err != nil {
		// Roll back so a bad token does not leave a dead server record behind.
		if created {
			_ = db.RemoveMCPServer(server.ID)
		} else if previous != "" {
			_ = db.SetMCPToken(server.ID, previous)
		}
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
	return nil
}

// midnightServerFor resolves the single server a Dot may use for a Midnight
// tool. The Dot's own `Midnight dot-<id>` record (endpoint ending /dot-<id>) is
// preferred; otherwise exactly one manually configured server may carry a
// current grant for the tool. A grant is honoured only while the server still
// advertises the tool at the granted fingerprint. Several candidates, a record
// that belongs to a different Dot, or a stale/missing grant are all refused.
func midnightServerFor(conversation task.Conversation, servers []task.MCPServer, tool string) (task.MCPServer, error) {
	byID := make(map[string]task.MCPServer, len(servers))
	for _, server := range servers {
		byID[server.ID] = server
	}
	label := "/dot-" + conversation.ID
	var own, other []task.MCPServer
	seen := map[string]bool{}
	granted := false
	for _, grant := range conversation.MCPToolGrants {
		if grant.ToolName != tool || seen[grant.ServerID] {
			continue
		}
		server, ok := byID[grant.ServerID]
		if !ok {
			continue
		}
		granted = true
		current := false
		for _, advertised := range server.Tools {
			if advertised.Name == tool && advertised.Fingerprint == grant.Fingerprint {
				current = true
			}
		}
		if !current {
			continue
		}
		seen[grant.ServerID] = true
		switch {
		case server.Name == "Midnight dot-"+conversation.ID && strings.HasSuffix(server.Endpoint, label):
			own = append(own, server)
		case strings.HasSuffix(server.Name, "dot-"+conversation.ID) || strings.Contains(server.Endpoint, "/dot-"):
			// A per-Dot record that is not this Dot's, or a mislabelled one.
			return task.MCPServer{}, fmt.Errorf("server %q is labelled for a different Dot; run `aether midnight enable %s`", server.Name, conversation.ID)
		default:
			other = append(other, server)
		}
	}
	switch {
	case len(own) == 1:
		return own[0], nil
	case len(own) > 1:
		return task.MCPServer{}, errors.New("ambiguous: several Midnight connections for this Dot; remove duplicates with `aether mcp remove`")
	case len(other) == 1:
		return other[0], nil
	case len(other) > 1:
		return task.MCPServer{}, fmt.Errorf("ambiguous: %d servers grant %q to this Dot; run `aether midnight enable %s` to select a dedicated connection", len(other), tool, conversation.ID)
	case granted:
		return task.MCPServer{}, fmt.Errorf("the grant for %q is stale; run `aether mcp refresh` and re-grant", tool)
	}
	return task.MCPServer{}, fmt.Errorf("tool %q is not granted to this Dot; run `aether midnight enable %s` (add --forget for forget)", tool, conversation.ID)
}

// memoryCommand performs user-initiated memory operations against Midnight.
func memoryCommand(db *store.Store, args []string) error {
	usage := errors.New("usage: aether memory <remember|recall|list|forget> <dot-id> ...\n  remember <dot-id> <text> [--derived] [--supersedes <id>] [--source-uri URI]\n  recall <dot-id> <query> [--limit N]\n  list <dot-id>\n  forget <dot-id> <memory-id>")
	if len(args) < 2 {
		return usage
	}
	positional, flags := splitFlags(args[1:])
	fs := flag.NewFlagSet("memory", flag.ContinueOnError)
	supersedes := fs.Int64("supersedes", 0, "memory id this fact replaces")
	sourceURI := fs.String("source-uri", "", "provenance URI for the memory")
	limit := fs.Int("limit", 5, "maximum recall results")
	derived := fs.Bool("derived", false, "store a derived (supersedable) fact with provenance")
	if err := fs.Parse(flags); err != nil || len(positional) < 1 {
		return usage
	}
	snapshot, err := db.SnapshotChecked()
	if err != nil {
		return err
	}
	var conversation task.Conversation
	found := false
	for _, candidate := range snapshot.Conversations {
		if candidate.ID == positional[0] {
			conversation, found = candidate, true
		}
	}
	if !found {
		return errors.New("Dot not found")
	}
	scope := dotScope(conversation.ID)
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
	server, err := midnightServerFor(conversation, snapshot.MCPServers, tool)
	if err != nil {
		return err
	}
	options, err := db.MCPConnectOptions(server.ID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := mcpclient.Connect(ctx, cliVersion, server.Endpoint, options...)
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
	// Runs whose owner process died are paused so they can be retried; runs
	// owned by a live Desktop or CLI process are reported and left alone.
	recovered, live, recoverErr := db.RecoverOrphanedRuns()
	if recoverErr != nil {
		return fmt.Errorf("check for interrupted runs: %w", recoverErr)
	}
	snapshot, snapErr := db.SnapshotChecked()
	if snapErr != nil {
		return snapErr
	}
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
	fmt.Printf("runs:        recovered=%d (owner process gone) active-elsewhere=%d\n", recovered, live)
	provider := model.NewProvider(model.FromEnvironment())
	ready, reason := provider.Readiness()
	fmt.Printf("model:       %s ready=%v %s\n", provider.Description(), ready, reason)
	problems := 0
	for _, server := range snapshot.MCPServers {
		start := time.Now()
		options, err := db.MCPConnectOptions(server.ID)
		var tools []task.MCPTool
		if err == nil {
			tools, err = discoverTools(context.Background(), server.Endpoint, options...)
		}
		if err != nil {
			problems++
			state := "UNREACHABLE"
			if errors.Is(err, mcpclient.ErrUnauthorized) {
				state = "REJECTED"
			}
			fmt.Printf("mcp %s %s %s (%v)\n", server.Name, state, server.Endpoint, err)
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
	return engine.WithSpecialistDelegationFactory(db, id, engine.WithGitHubWriteTools(db, id, providerForDot(id), service), func(delegateID string) model.Provider {
		// Delegated Dots never receive MCP tools.
		return engine.WithGitHubTools(db, delegateID, model.NewProvider(model.FromEnvironment()), service)
	}, nil)
}
