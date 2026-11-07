package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	mcpclient "aether-dots/internal/integrations/mcp"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) mcpSettingsPage(c *ui.Context, theme *ui.Theme) {
	if a.win != nil && !a.mcpDiscoveryScanned && !a.mcpDiscoveryLoading {
		a.startMCPDiscovery()
	}
	ui.Column(c.Key("mcp-settings-page")).FillWidth().MinWidth(0).Gap(16).Children(func() {
		card := ui.Column(c.Key("mcp-discovery-card")).FillWidth().MinWidth(0).Gap(12).Padding(17, 18).
			Radius(15).Background(theme.Surface).Border(1, theme.Border)
		card.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "Local MCP servers").FontSize(15).Bold()
					ui.Text(c, "AETHER searches this Mac's open local ports for MCP servers. No URL or server name needed.").
						FontSize(11).TextColor(theme.TextMuted).FillWidth()
				})
				label := "Find servers"
				if a.mcpDiscoveryScanned {
					label = "Scan again"
				}
				if a.mcpDiscoveryLoading {
					label = fmt.Sprintf("Searching %d%%…", a.mcpDiscoveryProgress)
				}
				if ui.PrimaryButton(c.Key("mcp-discover"), label).Padding(9, 13).Disabled(a.mcpDiscoveryLoading).Clicked() {
					a.startMCPDiscovery()
				}
			})
			if a.mcpDiscoveryLoading {
				ui.Text(c, "Checking local ports for Streamable HTTP MCP servers…").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			if a.mcpDiscoveryError != "" {
				ui.Text(c, a.mcpDiscoveryError).FontSize(11).TextColor(theme.Danger).FillWidth()
			}
			ui.Text(c, "Only this Mac's 127.0.0.1 and ::1 addresses are checked. AETHER tests the common /mcp and / endpoints; it never scans the internet or starts a server command.").
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
		})

		ui.Column(c.Key("mcp-connected-servers")).FillWidth().MinWidth(0).Gap(10).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "Discovered servers").FontSize(14).Bold()
				ui.Spacer(c)
				if len(a.snapshot.MCPServers) > 0 {
					ui.Text(c, fmt.Sprintf("%d", len(a.snapshot.MCPServers))).FontSize(10).TextColor(theme.TextMuted)
				}
			})
			if len(a.snapshot.MCPServers) == 0 {
				message := "Open this tab to search for MCP servers running on this Mac."
				if a.mcpDiscoveryLoading {
					message = "Searching this Mac for running MCP servers…"
				} else if a.mcpDiscoveryScanned && a.mcpDiscoveryError == "" {
					message = "No MCP servers found. Start a server with Streamable HTTP on localhost, then choose Scan again. Servers that only use stdio are not discoverable here."
				} else if a.mcpDiscoveryError != "" {
					message = "No validated MCP servers are connected yet."
				}
				ui.Column(c.Key("mcp-empty-state")).FillWidth().Gap(4).Padding(15, 16).Radius(12).
					Background(theme.Surface).Border(1, theme.Border).Children(func() {
					ui.Text(c, message).FontSize(11).TextColor(theme.TextMuted).FillWidth()
				})
				return
			}
			for _, server := range a.snapshot.MCPServers {
				server := server
				ui.Row(c.Key("mcp-server-"+server.ID)).FillWidth().AlignItems(ui.Center).Gap(10).Padding(12, 14).
					Radius(12).Background(theme.Surface).Border(1, theme.Border).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, server.Name).FontSize(12).Bold()
						ui.Text(c, mcpServerLocalLabel(server.Endpoint)+fmt.Sprintf(" · %d tools · %d Dots granted", len(server.Tools), mcpGrantingConversationCount(a.snapshot, server.ID))).
							FontSize(10).TextColor(theme.TextMuted).FillWidth()
					})
					if ui.Button(c.Key("mcp-disconnect-"+server.ID), "Disconnect").Padding(7, 10).
						Disabled(a.mcpDiscoveryLoading).Clicked() {
						if err := a.db.RemoveMCPServer(server.ID); err != nil {
							a.mcpDiscoveryError = "Couldn't disconnect this server. Try again."
						} else {
							a.mcpDiscoveryError = ""
							a.refresh()
						}
					}
				})
			}
		})

		if len(a.snapshot.MCPServers) > 0 {
			ui.Column(c.Key("mcp-next-step")).FillWidth().Gap(6).Padding(14, 16).Radius(12).
				Background(theme.Accent.Alpha(0.08)).Children(func() {
				ui.Text(c, "Choose which Dot can use a server").FontSize(12).Bold()
				ui.Text(c, "Open a Dot's Customize → MCP tools section to grant exact tools. Each Dot keeps its own access.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
				if a.selectedConversationID != "" {
					if ui.Button(c.Key("mcp-open-dot-access"), "Choose tools for selected Dot").Padding(8, 11).Clicked() {
						a.settingsOpen = false
						a.openConversationProfile()
					}
				}
			})
		}
	})
}

func (a *app) startMCPDiscovery() {
	if a.mcpDiscoveryLoading || a.win == nil {
		return
	}
	a.mcpDiscoveryScanned = true
	a.mcpDiscoveryLoading = true
	a.mcpDiscoveryProgress = 0
	a.mcpDiscoveryError = ""
	win := a.win
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	var lastProgress int
	go func() {
		defer cancel()
		discovered, discoveryErr := mcpclient.DiscoverLocalServers(ctx, func(progress int) {
			if progress <= lastProgress || progress%2 != 0 {
				return
			}
			lastProgress = progress
			win.Update(func() {
				a.mcpDiscoveryProgress = progress
				a.refresh()
			})
		})
		known := make(map[string]string)
		for _, server := range a.db.Snapshot().MCPServers {
			known[server.Endpoint] = server.ID
		}
		connected, skipped := 0, 0
		seen := make(map[string]bool)
		for _, candidate := range discovered {
			endpoint, err := mcpclient.NormalizeEndpoint(candidate.Endpoint)
			if err != nil {
				skipped++
				continue
			}
			tools, err := mcpclient.ConvertTools(candidate.Tools)
			if err != nil {
				skipped++
				continue
			}
			portKey := strings.ToLower(strings.TrimSpace(candidate.Name)) + ":" + mcpServerLocalLabel(endpoint)
			if seen[portKey] {
				continue
			}
			seen[portKey] = true
			serverID := known[endpoint]
			if serverID == "" {
				server, addErr := a.db.AddMCPServer(mcpServerSafeName(candidate.Name, endpoint), endpoint)
				if addErr != nil {
					skipped++
					continue
				}
				serverID = server.ID
			}
			if err := a.db.RefreshMCPServerTools(serverID, tools); err != nil {
				skipped++
				continue
			}
			known[endpoint] = serverID
			connected++
		}
		finish := func() {
			a.mcpDiscoveryLoading = false
			a.mcpDiscoveryProgress = 100
			a.refresh()
			switch {
			case discoveryErr != nil:
				a.mcpDiscoveryError = "Local scan timed out. Some ports may not have been checked; choose Scan again to retry."
			case skipped > 0:
				a.mcpDiscoveryError = "Some local server tool details couldn't be validated. No new tools were enabled."
			case connected > 0:
				a.mcpDiscoveryError = ""
			case len(a.snapshot.MCPServers) == 0:
				a.mcpDiscoveryError = ""
			}
		}
		win.Update(finish)
	}()
}

func mcpServerLocalLabel(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Port() == "" {
		return "This Mac"
	}
	return "This Mac · port " + parsed.Port()
}

func mcpServerSafeName(name, endpoint string) string {
	var cleaned strings.Builder
	for _, char := range name {
		if unicode.IsControl(char) {
			cleaned.WriteByte(' ')
		} else {
			cleaned.WriteRune(char)
		}
	}
	name = strings.Join(strings.Fields(cleaned.String()), " ")
	if name == "" {
		name = "Local MCP server on " + mcpServerLocalLabel(endpoint)
	}
	if len(name) > task.MaxMCPServerNameBytes {
		name = truncateUTF8Bytes(name, task.MaxMCPServerNameBytes)
	}
	return name
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
