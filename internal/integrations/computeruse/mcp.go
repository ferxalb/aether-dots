package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxMCPLineBytes = 8 * 1024 * 1024

type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan struct{}
	closed chan struct{}

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan mcpResponse
	nextID  atomic.Int64
	closeOnce sync.Once
	failOnce  sync.Once
}

type mcpResponse struct {
	result json.RawMessage
	err    error
}

type rpcFrame struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallResult struct {
	IsError   bool            `json:"isError"`
	Content   []mcpContent    `json:"content"`
	Structured json.RawMessage `json:"structuredContent"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newMCPClient(ctx context.Context, driverPath, socket string) (*mcpClient, error) {
	cmd := exec.Command(driverPath, "mcp", "--socket", socket)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	client := &mcpClient{
		cmd: cmd, stdin: stdin, done: make(chan struct{}), closed: make(chan struct{}),
		pending: make(map[string]chan mcpResponse),
	}
	go client.readLoop(stdout)
	go func() {
		_ = cmd.Wait()
		client.failPending(errors.New("Cua Driver MCP connection closed"))
	}()
	initCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, err = client.request(initCtx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "aether-dots", "version": "0.1.0"},
	})
	if err != nil {
		client.Close()
		return nil, err
	}
	if err := client.notify("notifications/initialized", map[string]any{}); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
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
	var result toolCallResult
	raw, err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, errors.New("Cua Driver returned an invalid MCP tool result")
	}
	return result, nil
}

func (c *mcpClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := strconv.FormatInt(c.nextID.Add(1), 10)
	response := make(chan mcpResponse, 1)
	c.mu.Lock()
	if !c.Alive() {
		c.mu.Unlock()
		return nil, errors.New("Cua Driver MCP connection is closed")
	}
	c.pending[id] = response
	c.mu.Unlock()
	frame := map[string]any{"jsonrpc": "2.0", "id": json.Number(id), "method": method, "params": params}
	if err := c.writeFrame(frame); err != nil {
		c.removePending(id)
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	case <-c.done:
		c.removePending(id)
		return nil, errors.New("Cua Driver MCP connection closed")
	case reply := <-response:
		if reply.err != nil {
			return nil, reply.err
		}
		return reply.result, nil
	}
}

func (c *mcpClient) notify(method string, params any) error {
	return c.writeFrame(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *mcpClient) writeFrame(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(b); err != nil {
		return errors.New("could not write to Cua Driver MCP connection")
	}
	return nil
}

func (c *mcpClient) readLoop(reader io.Reader) {
	defer close(c.done)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxMCPLineBytes)
	for scanner.Scan() {
		var frame rpcFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		if len(frame.ID) == 0 || string(frame.ID) == "null" {
			continue
		}
		key := strings.TrimSpace(string(frame.ID))
		c.mu.Lock()
		ch := c.pending[key]
		delete(c.pending, key)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		if frame.Error != nil {
			ch <- mcpResponse{err: fmt.Errorf("Cua Driver MCP error %d", frame.Error.Code)}
			continue
		}
		ch <- mcpResponse{result: frame.Result}
	}
}

func (c *mcpClient) removePending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *mcpClient) failPending(err error) {
	c.failOnce.Do(func() {
		c.mu.Lock()
		for id, ch := range c.pending {
			delete(c.pending, id)
			ch <- mcpResponse{err: err}
		}
		c.mu.Unlock()
	})
}

func (c *mcpClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		c.failPending(errors.New("Cua Driver MCP connection closed"))
	})
}
