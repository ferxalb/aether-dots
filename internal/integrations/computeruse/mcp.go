package computeruse

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	clientbase "aether-dots/internal/integrations/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpClient struct {
	session *mcpsdk.ClientSession
	done    chan struct{}

	closeOnce sync.Once
}

type toolCallResult struct {
	IsError    bool
	Text       string
	Structured any
}

func newMCPClient(ctx context.Context, driverPath, socket string) (*mcpClient, error) {
	command := exec.Command(driverPath, "mcp", "--socket", socket)
	command.Env = driverEnvironment(os.Environ(), "")
	client := clientbase.NewClient("0.1.0")
	session, err := client.Connect(ctx, &mcpsdk.CommandTransport{
		Command: command, TerminateDuration: time.Second,
	}, &mcpsdk.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	if err != nil {
		return nil, errors.New("could not initialize the Cua Driver MCP session")
	}
	mcp := &mcpClient{session: session, done: make(chan struct{})}
	go func() {
		_ = session.Wait()
		close(mcp.done)
	}()
	return mcp, nil
}

func (c *mcpClient) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *mcpClient) CallTool(ctx context.Context, name string, args map[string]any) (toolCallResult, error) {
	var output toolCallResult
	if !c.Alive() {
		return output, errors.New("Cua Driver MCP connection is closed")
	}
	result, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return output, err
	}
	output.IsError = result.IsError
	output.Structured = result.StructuredContent
	var text []string
	for _, content := range result.Content {
		if value, ok := content.(*mcpsdk.TextContent); ok && value.Text != "" {
			text = append(text, value.Text)
		}
	}
	output.Text = strings.Join(text, "\n")
	return output, nil
}

func (c *mcpClient) Close() {
	c.closeOnce.Do(func() { _ = c.session.Close() })
}
