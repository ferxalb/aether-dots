package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"
)

// Errors returned by Client. Network trouble is ErrUnavailable so callers can
// keep the current state and retry later.
var (
	ErrNotPermitted = errors.New("license key is revoked, disabled or expired, or has reached its device limit")
	ErrNotFound     = errors.New("license key or activation was not found")
	ErrUnavailable  = errors.New("the license service could not be reached")
	ErrMismatch     = errors.New("the license service returned a response for a different key or organization")
)

const (
	requestTimeout   = 15 * time.Second
	maxResponseBytes = 64 << 10
)

// Client talks to Polar's public customer-portal license-key endpoints. They
// need no credential; only the key, the organization ID, a device label and
// the activation ID are sent.
type Client struct {
	cfg  Config
	http *http.Client
	now  func() time.Time
}

// NewClient returns a client for cfg's fixed base URL. Redirects are refused.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:       requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now: time.Now,
	}
}

// DeviceLabel names an activation without identifying the machine or user.
func DeviceLabel() string {
	return "AETHER " + runtime.GOOS + "/" + runtime.GOARCH
}

type activateRequest struct {
	Key            string `json:"key"`
	OrganizationID string `json:"organization_id"`
	Label          string `json:"label"`
}

type activationResponse struct {
	ID           string `json:"id"`
	LicenseKeyID string `json:"license_key_id"`
}

type validateRequest struct {
	Key            string `json:"key"`
	OrganizationID string `json:"organization_id"`
	ActivationID   string `json:"activation_id"`
}

type validatedKey struct {
	OrganizationID string     `json:"organization_id"`
	Key            string     `json:"key"`
	DisplayKey     string     `json:"display_key"`
	Status         string     `json:"status"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Activation     *struct {
		ID string `json:"id"`
	} `json:"activation"`
}

type deactivateRequest struct {
	Key            string `json:"key"`
	OrganizationID string `json:"organization_id"`
	ActivationID   string `json:"activation_id"`
}

// Activate registers this device for key and validates it. On success the
// returned State is ready to save.
func (c *Client) Activate(ctx context.Context, key string) (*State, error) {
	key, err := NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	var act activationResponse
	if err := c.post(ctx, "/v1/customer-portal/license-keys/activate", activateRequest{
		Key: key, OrganizationID: c.cfg.OrganizationID, Label: DeviceLabel(),
	}, http.StatusOK, &act); err != nil {
		return nil, err
	}
	if !isUUID(act.ID) {
		return nil, fmt.Errorf("%w: activation has no valid ID", ErrMismatch)
	}
	s := &State{Version: stateVersion, Key: key, ActivationID: act.ID, ActivatedAt: c.now().UTC()}
	if err := c.Validate(ctx, s); err != nil {
		// Give the device slot back; the key may be fine on another try.
		_ = c.Deactivate(ctx, s)
		return nil, err
	}
	return s, nil
}

// Validate checks s online and updates it. On ErrUnavailable s is unchanged.
// When Polar no longer recognises the key or activation, s.Revoked is set
// (unless the key simply passed its expiry) and ErrNotFound is returned.
func (c *Client) Validate(ctx context.Context, s *State) error {
	if s == nil || s.Key == "" || s.ActivationID == "" {
		return ErrNotFound
	}
	var v validatedKey
	err := c.post(ctx, "/v1/customer-portal/license-keys/validate", validateRequest{
		Key: s.Key, OrganizationID: c.cfg.OrganizationID, ActivationID: s.ActivationID,
	}, http.StatusOK, &v)
	now := c.now().UTC()
	if errors.Is(err, ErrNotFound) {
		// Polar answers 404 for revoked, disabled and expired keys alike.
		// A key past its own expiry is not treated as revoked, so its paid
		// period still counts for the fallback rule.
		if s.ExpiresAt == nil || now.Before(*s.ExpiresAt) {
			s.Revoked = true
		}
		return err
	}
	if err != nil {
		return err
	}
	if v.OrganizationID != c.cfg.OrganizationID || v.Key != s.Key ||
		(v.Activation != nil && v.Activation.ID != s.ActivationID) {
		return ErrMismatch
	}
	if v.Status != "granted" {
		s.Revoked = true
		return ErrNotPermitted
	}
	if v.ExpiresAt != nil {
		t := v.ExpiresAt.UTC()
		s.ExpiresAt = &t
	} else {
		s.ExpiresAt = nil
	}
	s.DisplayKey = v.DisplayKey
	s.LastValidatedAt = now
	s.Revoked = false
	return nil
}

// Deactivate frees this device's activation slot.
func (c *Client) Deactivate(ctx context.Context, s *State) error {
	if s == nil || s.Key == "" || s.ActivationID == "" {
		return nil
	}
	return c.post(ctx, "/v1/customer-portal/license-keys/deactivate", deactivateRequest{
		Key: s.Key, OrganizationID: c.cfg.OrganizationID, ActivationID: s.ActivationID,
	}, http.StatusNoContent, nil)
}

func (c *Client) post(ctx context.Context, path string, body any, want int, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AETHER")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("%w: response too large", ErrUnavailable)
	}
	switch {
	case resp.StatusCode == want:
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%w: malformed response", ErrUnavailable)
		}
		return nil
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotPermitted
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest:
		return errors.New("the license service rejected the request; check the key")
	default:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
}
