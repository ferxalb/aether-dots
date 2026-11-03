package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// BuildRelayOrigin is an AETHER-owned Cloudflare Worker origin supplied by the
// release build. The desktop never accepts a user-entered remote destination.
var BuildRelayOrigin string

const maxResponseBytes = 64 * 1024

type DeviceCredential struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type Pairing struct {
	DeviceID string `json:"deviceId"`
	Code     string `json:"code"`
}

type Command struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type Client struct {
	base   *url.URL
	http   *http.Client
	token  string
	device string
}

func New(origin string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("the AETHER relay endpoint is not configured for this build")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Port() != "" || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".workers.dev") {
		return nil, errors.New("the AETHER relay must use its pinned Cloudflare Workers endpoint")
	}
	return &Client{
		base: parsed,
		http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func NewDevelopment(origin string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed == nil || parsed.User != nil || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid local relay endpoint")
	}
	if parsed.Scheme != "http" || parsed.Port() != "5173" || !isLoopback(parsed.Hostname()) {
		return nil, errors.New("development relay access is limited to the loopback Cloudflare Worker on port 5173")
	}
	return &Client{
		base: parsed,
		http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func Configured() (*Client, error) {
	if os.Getenv("AETHER_RELAY_DEV") == "1" {
		return NewDevelopment(os.Getenv("AETHER_RELAY_URL"))
	}
	return New(BuildRelayOrigin)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) WithDevice(credential DeviceCredential) *Client {
	copy := *c
	copy.device = credential.ID
	copy.token = credential.Token
	return &copy
}

func (c *Client) StartPairing(ctx context.Context, name string) (Pairing, error) {
	var result Pairing
	err := c.request(ctx, http.MethodPost, "/api/v1/pairings", map[string]string{"deviceName": name}, "", &result)
	return result, err
}

func (c *Client) PairingStatus(ctx context.Context, code string) (string, error) {
	var result struct {
		State string `json:"state"`
	}
	err := c.request(ctx, http.MethodPost, "/api/v1/pairings/status", map[string]string{"code": code}, "", &result)
	return result.State, err
}

func (c *Client) CancelPairing(ctx context.Context, code string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/pairings/cancel", map[string]string{"code": code}, "", nil)
}

func (c *Client) ConfirmPairing(ctx context.Context, code string) (DeviceCredential, error) {
	var result struct {
		ID    string `json:"deviceId"`
		Token string `json:"deviceToken"`
	}
	err := c.request(ctx, http.MethodPost, "/api/v1/pairings/confirm", map[string]string{"code": code}, "", &result)
	return DeviceCredential{ID: result.ID, Token: result.Token}, err
}

func (c *Client) Heartbeat(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "/api/v1/devices/heartbeat", nil, c.token, nil)
}

func (c *Client) NextCommand(ctx context.Context) (*Command, error) {
	var result struct {
		Command *Command `json:"command"`
	}
	err := c.request(ctx, http.MethodGet, "/api/v1/desktop/commands/next", nil, c.token, &result)
	return result.Command, err
}

func (c *Client) CompleteCommand(ctx context.Context, id, state string, result any) error {
	body := map[string]any{"state": state}
	if state == "succeeded" {
		body["result"] = result
	}
	return c.request(ctx, http.MethodPost, "/api/v1/desktop/commands/"+url.PathEscape(id)+"/complete", body, c.token, nil)
}

func (c *Client) Disconnect(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "/api/v1/desktop/disconnect", nil, c.token, nil)
}

func (c *Client) request(ctx context.Context, method, path string, body any, deviceToken string, out any) error {
	endpoint := *c.base
	endpoint.Path = path
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return errors.New("could not encode the relay request")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return errors.New("could not prepare the relay request")
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if deviceToken != "" {
		request.Header.Set("X-Aether-Device-Token", deviceToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return errors.New("could not reach the AETHER relay")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return errors.New("the AETHER relay response was invalid or too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var remoteError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &remoteError) == nil && remoteError.Error != "" {
			return fmt.Errorf("%s", remoteError.Error)
		}
		return fmt.Errorf("the AETHER relay returned status %d", response.StatusCode)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("the AETHER relay response could not be read")
	}
	return nil
}
