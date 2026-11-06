package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	mcpclient "aether-dots/internal/integrations/mcp"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) mcpIntegrationCard(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c.Key("integration-mcp-card")).FillWidth().MinWidth(0).Gap(14).Padding(17, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "MCP servers").FontSize(15).Bold()
			ui.Text(c, "Connect to MCP servers already running on this Mac. AETHER accepts loopback HTTP endpoints only and never starts a server command.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		if a.mcpServerError != "" {
			ui.Text(c, a.mcpServerError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}
		ui.Divider(c)
		ui.Field(c, "Server name", func() {
			ui.TextInputBase(c.Key("mcp-server-name"), &a.mcpServerNameDraft).
				Placeholder("e.g. Local knowledge tools").FillWidth().Label("MCP server name")
		})
		ui.Field(c, "Loopback endpoint", func() {
			ui.TextInputBase(c.Key("mcp-server-endpoint"), &a.mcpServerEndpointDraft).
				Placeholder("http://127.0.0.1:8765/mcp").FillWidth().Label("MCP loopback URL")
		})
		if ui.PrimaryButton(c.Key("mcp-server-add"), "Connect and review tools").Padding(9, 13).
			Disabled(a.mcpServerLoadingID != "").Clicked() {
			a.addMCPServer()
		}
		if len(a.snapshot.MCPServers) == 0 {
			ui.Text(c, "No MCP servers connected.").FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		for _, server := range a.snapshot.MCPServers {
			server := server
			ui.Column(c.Key("mcp-server-"+server.ID)).FillWidth().Gap(8).Padding(12, 13).Radius(11).
				Background(theme.Background).Border(1, theme.Border).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, server.Name).FontSize(12).Bold()
						ui.Text(c, server.Endpoint).FontSize(10).TextColor(theme.TextMuted).FillWidth()
					})
					busy := a.mcpServerLoadingID == server.ID
					label := "Refresh tools"
					if busy {
						label = "Connecting…"
					}
					if ui.Button(c.Key("mcp-refresh-"+server.ID), label).Padding(7, 9).Disabled(a.mcpServerLoadingID != "").Clicked() {
						a.refreshMCPServerTools(server.ID)
					}
					if ui.Button(c.Key("mcp-remove-"+server.ID), "Remove").Padding(7, 9).Disabled(a.mcpServerLoadingID != "").Clicked() {
						if err := a.db.RemoveMCPServer(server.ID); err != nil {
							a.mcpServerError = err.Error()
						} else {
							a.mcpServerError = ""
							a.refresh()
						}
					}
				})
				ui.Text(c, fmt.Sprintf("%d discovered tools · %d Dots have their own grants", len(server.Tools), mcpGrantingConversationCount(a.snapshot, server.ID))).
					FontSize(10).TextColor(theme.TextMuted).FillWidth()
			})
		}
		ui.Text(c, "Review every server and tool before granting it. A server can act within its own scope; tool annotations are hints only. Dot-specific grants and a model-data disclosure are required, and MCP tools are off for Routines, delegated Dots, and iPhone messages.").
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	})
}

func (a *app) addMCPServer() {
	if a.mcpServerLoadingID != "" {
		return
	}
	endpoint, err := mcpclient.NormalizeEndpoint(a.mcpServerEndpointDraft)
	if err != nil {
		a.mcpServerError = err.Error()
		return
	}
	server, err := a.db.AddMCPServer(a.mcpServerNameDraft, endpoint)
	if err != nil {
		a.mcpServerError = err.Error()
		return
	}
	a.mcpServerNameDraft = ""
	a.mcpServerEndpointDraft = ""
	a.mcpServerError = ""
	a.refresh()
	a.refreshMCPServerTools(server.ID)
}

func (a *app) refreshMCPServerTools(id string) {
	if a.mcpServerLoadingID != "" {
		return
	}
	var server *task.MCPServer
	for i := range a.snapshot.MCPServers {
		if a.snapshot.MCPServers[i].ID == id {
			copy := a.snapshot.MCPServers[i]
			server = &copy
			break
		}
	}
	if server == nil {
		a.mcpServerError = "MCP server not found. Refresh Integrations and try again."
		return
	}
	a.mcpServerLoadingID = id
	a.mcpServerRequest++
	request := a.mcpServerRequest
	a.mcpServerError = ""
	win := a.win
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go func() {
		defer cancel()
		session, err := mcpclient.Connect(ctx, "0.1.0", server.Endpoint)
		var tools []task.MCPTool
		if err == nil {
			remoteTools, listErr := session.ListTools(ctx)
			if listErr != nil {
				err = listErr
			} else {
				tools, err = mcpclient.ConvertTools(remoteTools)
			}
			_ = session.Close()
		}
		if err == nil {
			err = a.db.RefreshMCPServerTools(server.ID, tools)
		}
		finish := func() {
			if a.mcpServerRequest != request {
				return
			}
			a.mcpServerLoadingID = ""
			a.refresh()
			if err != nil {
				a.mcpServerError = fmt.Sprintf("Couldn't refresh %s: %v", server.Name, err)
				return
			}
			a.adt.Success("MCP tools refreshed", "Review tool names, descriptions, and schemas in the Dot's Customize panel before granting access.")
		}
		if win == nil {
			finish()
			return
		}
		win.Update(finish)
	}()
}

func mcpGrantingConversationCount(snapshot task.Database, serverID string) int {
	count := 0
	for _, conversation := range snapshot.Conversations {
		for _, grant := range conversation.MCPToolGrants {
			if grant.ServerID == serverID {
				count++
				break
			}
		}
	}
	return count
}

func toggleMCPToolGrant(grants []task.MCPToolGrant, serverID string, tool task.MCPTool) ([]task.MCPToolGrant, error) {
	for i, grant := range grants {
		if grant.ServerID == serverID && grant.ToolName == tool.Name {
			return append(grants[:i], grants[i+1:]...), nil
		}
	}
	if len(grants) >= task.MaxMCPDotToolGrants {
		return grants, fmt.Errorf("a Dot can be granted at most %d MCP tools", task.MaxMCPDotToolGrants)
	}
	return append(grants, task.MCPToolGrant{ServerID: serverID, ToolName: tool.Name, Fingerprint: tool.Fingerprint}), nil
}

func hasMCPToolGrant(grants []task.MCPToolGrant, serverID, toolName, fingerprint string) bool {
	for _, grant := range grants {
		if grant.ServerID == serverID && grant.ToolName == toolName && grant.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

func formatMCPToolSchema(tool task.MCPTool) string {
	encoded, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return "schema is not valid JSON"
	}
	return string(encoded)
}

type mcpProfileTool struct {
	server task.MCPServer
	tool   task.MCPTool
}

func filterMCPProfileTools(servers []task.MCPServer, query string) []mcpProfileTool {
	query = strings.ToLower(strings.TrimSpace(query))
	var matches []mcpProfileTool
	for _, server := range servers {
		for _, tool := range server.Tools {
			if query != "" && !strings.Contains(strings.ToLower(server.Name+" "+tool.Name+" "+tool.Description), query) {
				continue
			}
			matches = append(matches, mcpProfileTool{server: server, tool: tool})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].server.Name != matches[j].server.Name {
			return matches[i].server.Name < matches[j].server.Name
		}
		return matches[i].tool.Name < matches[j].tool.Name
	})
	return matches
}
