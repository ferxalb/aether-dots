// Package mcp implements AETHER's loopback-only MCP client. Server processes
// are started and managed by the user; AETHER never launches MCP commands.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const implementationName = "aether-dots"
const maxMCPHTTPResponseBytes = 2 << 20

// ErrUnauthorized means the server answered but rejected (or lacked) credentials.
var ErrUnauthorized = errors.New("MCP server rejected the credentials (HTTP 401/403): the token is missing, revoked, or expired")

var ErrToolListLimitExceeded = errors.New("MCP tool list exceeded the configured limit")

// NewClient creates an MCP client that advertises no optional capabilities.
// In particular, it does not advertise roots, sampling, or elicitation.
func NewClient(version string) *mcpsdk.Client {
	if version == "" {
		version = "dev"
	}
	return mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: implementationName, Version: version},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
}

// NormalizeEndpoint accepts only explicit loopback HTTP endpoints. MCP server
// commands and remote network destinations are intentionally unsupported.
func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("MCP endpoint must be a loopback HTTP URL without credentials, query, or fragment")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || (!ip.Equal(net.ParseIP("127.0.0.1")) && !ip.Equal(net.ParseIP("::1"))) {
		return "", errors.New("MCP endpoint must use the 127.0.0.1 or ::1 loopback address")
	}
	port := u.Port()
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("MCP endpoint must include a valid port")
	}
	port = strconv.Itoa(portNumber)
	if u.Opaque != "" || strings.Contains(u.Path, "\\") {
		return "", errors.New("MCP endpoint path is invalid")
	}
	u.Host = net.JoinHostPort(ip.String(), port)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	if u.Path == "" {
		u.Path = "/mcp"
	}
	return u.String(), nil
}

// Session owns one short-lived MCP connection for a single Dot run.
type Session struct {
	session *mcpsdk.ClientSession
	client  *http.Client
}

// Option customises Connect.
type Option func(*connectOptions)

type connectOptions struct{ bearer string }

// WithBearer authenticates every request to the configured endpoint with a
// bearer token. The token is attached only by this client's transport, never
// logged, and never forwarded because redirects are disabled.
func WithBearer(token string) Option {
	return func(o *connectOptions) { o.bearer = strings.TrimSpace(token) }
}

type DiscoveredServer struct {
	Endpoint string
	Name     string
	Version  string
	Tools    []*mcpsdk.Tool
}

func Connect(ctx context.Context, version, endpoint string, opts ...Option) (*Session, error) {
	var options connectOptions
	for _, opt := range opts {
		opt(&options)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = "dev"
	}
	status := &statusRecorder{}
	client := loopbackHTTPClient(endpoint, options.bearer)
	client.Transport = statusTransport{base: client.Transport, status: status}
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: client,
		MaxRetries: -1, DisableStandaloneSSE: true,
		MaxEventSize: maxMCPHTTPResponseBytes,
	}
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := NewClient(version).Connect(connectCtx, transport, nil)
	if err != nil || session == nil {
		client.CloseIdleConnections()
		switch status.last.Load() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, ErrUnauthorized
		}
		return nil, errors.New("could not connect to the loopback MCP server")
	}
	return &Session{session: session, client: client}, nil
}

func (s *Session) ListTools(ctx context.Context) ([]*mcpsdk.Tool, error) {
	if s == nil || s.session == nil {
		return nil, errors.New("MCP session is closed")
	}
	var tools []*mcpsdk.Tool
	cursor := ""
	for page := 0; page < 8; page++ {
		result, err := s.session.ListTools(ctx, &mcpsdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		if result == nil {
			return nil, errors.New("MCP server returned an empty tool list")
		}
		if len(tools)+len(result.Tools) > 64 {
			return nil, fmt.Errorf("%w: more than 64 tools", ErrToolListLimitExceeded)
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		cursor = result.NextCursor
	}
	return nil, fmt.Errorf("%w: more than 8 pages", ErrToolListLimitExceeded)
}

func (s *Session) ServerIdentity() (name, version string) {
	if s == nil || s.session == nil {
		return "", ""
	}
	result := s.session.InitializeResult()
	if result == nil || result.ServerInfo == nil {
		return "", ""
	}
	return result.ServerInfo.Name, result.ServerInfo.Version
}

func (s *Session) CallTool(ctx context.Context, name string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	if s == nil || s.session == nil {
		return nil, errors.New("MCP session is closed")
	}
	result, err := s.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("MCP server returned an empty tool result")
	}
	return result, nil
}

func (s *Session) Close() error {
	if s == nil || s.session == nil {
		return nil
	}
	err := s.session.Close()
	s.session = nil
	if s.client != nil {
		s.client.CloseIdleConnections()
		s.client = nil
	}
	return err
}

func loopbackHTTPClient(endpoint, bearer string) *http.Client {
	u, _ := url.Parse(endpoint)
	allowedHost := u.Hostname()
	allowedPort := u.Port()
	baseTransport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host != allowedHost || port != allowedPort {
			return nil, errors.New("MCP request attempted to leave its configured loopback endpoint")
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, errors.New("MCP request target is not loopback")
		}
		return (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, address)
	}}
	var transport http.RoundTripper = limitedResponseTransport{base: baseTransport, limit: maxMCPHTTPResponseBytes}
	if bearer != "" {
		transport = bearerTransport{base: transport, token: bearer}
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("MCP endpoint redirects are disabled")
	}}
}

type statusRecorder struct{ last atomic.Int32 }

type statusTransport struct {
	base   http.RoundTripper
	status *statusRecorder
}

func (t statusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err == nil {
		t.status.last.Store(int32(response.StatusCode))
	}
	return response, err
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

type limitedResponseTransport struct {
	base  http.RoundTripper
	limit int64
}

func (t limitedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > t.limit {
		_ = response.Body.Close()
		return nil, errors.New("MCP response exceeds the 2 MiB limit")
	}
	response.Body = &limitedResponseBody{body: response.Body, remaining: t.limit}
	return response, nil
}

type limitedResponseBody struct {
	body      io.ReadCloser
	remaining int64
	checked   bool
}

func (b *limitedResponseBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if b.remaining > 0 {
		if int64(len(buffer)) > b.remaining {
			buffer = buffer[:b.remaining]
		}
		n, err := b.body.Read(buffer)
		b.remaining -= int64(n)
		return n, err
	}
	if b.checked {
		return 0, errors.New("MCP response exceeds the 2 MiB limit")
	}
	b.checked = true
	var extra [1]byte
	n, err := b.body.Read(extra[:])
	if n > 0 {
		return 0, errors.New("MCP response exceeds the 2 MiB limit")
	}
	return 0, err
}

func (b *limitedResponseBody) Close() error { return b.body.Close() }
