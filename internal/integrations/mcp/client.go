// Package mcp provides the inert starting point for AETHER's MCP client
// integration. It selects no transport or destination and does not connect.
package mcp

import mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

const implementationName = "aether-dots"

// NewClient creates an unconnected MCP client with no advertised capabilities.
// An empty version is reported as "dev"; release builds can pass their own.
func NewClient(version string) *mcpsdk.Client {
	if version == "" {
		version = "dev"
	}
	return mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: implementationName, Version: version},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
}
