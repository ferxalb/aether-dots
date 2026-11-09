package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DeviceCodeURL         = "https://github.com/login/device/code"
	AccessTokenURL        = "https://github.com/login/oauth/access_token"
	DeviceVerificationURL = "https://github.com/login/device"
	DeviceGrantType       = "urn:ietf:params:oauth:grant-type:device_code"
	maxOAuthResponseBytes = 64 * 1024
	refreshBeforeExpiry   = 2 * time.Minute
)

var (
	ErrNotConnected            = errors.New("GitHub is not connected")
	ErrSessionExpired          = errors.New("GitHub session expired; reconnect GitHub")
	ErrAuthorizationInProgress = errors.New("GitHub authorization is already in progress")
	ErrInvalidClient           = errors.New("GitHub App client ID is missing or invalid")
	ErrInvalidDeviceResponse   = errors.New("GitHub returned an invalid device authorization response")
	ErrAuthorizationDenied     = errors.New("GitHub authorization was denied")
	ErrDeviceCodeExpired       = errors.New("GitHub device code expired; start a new connection")
	ErrDeviceFlowDisabled      = errors.New("GitHub Device Flow is disabled for this app; enable it in the GitHub App registration")
	ErrCredentialsCorrupt      = errors.New("saved GitHub credentials could not be read; reconnect GitHub")
)

// Credentials are serialized only by the credential-store adapter. Never add
// this value to task/store or preference models.
type Credentials struct {
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at,omitempty"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`
}

type CredentialStore interface {
	Load(context.Context) (Credentials, error)
	Save(context.Context, Credentials) error
	Delete(context.Context) error
}

type DeviceAuthorization struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// PendingError means GitHub has not completed the user's authorization yet.
// Interval includes GitHub's five-second slow_down increase when applicable.
type PendingError struct {
	Interval time.Duration
	SlowDown bool
}

func (e *PendingError) Error() string { return "GitHub authorization is still pending" }

type oauthHTTPError struct{ statusCode int }

func (e *oauthHTTPError) Error() string {
	return fmt.Sprintf("GitHub authorization service returned HTTP %d", e.statusCode)
}

type oauthResponse struct {
	DeviceCode            string `json:"device_code"`
	UserCode              string `json:"user_code"`
	VerificationURI       string `json:"verification_uri"`
	ExpiresIn             int64  `json:"expires_in"`
	Interval              int64  `json:"interval"`
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	TokenType             string `json:"token_type"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
}

// BeginDeviceAuthorization requests one user code from GitHub. The client ID
// is public configuration, not a secret; GitHub App client secrets are never
// required or accepted by this flow.
func BeginDeviceAuthorization(ctx context.Context, clientID string, httpClient *http.Client) (DeviceAuthorization, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || len(clientID) > 256 || strings.ContainsAny(clientID, "\r\n\x00") {
		return DeviceAuthorization{}, ErrInvalidClient
	}
	form := url.Values{"client_id": {clientID}, "scope": {"offline_access"}}
	var response oauthResponse
	if err := postOAuthJSON(ctx, httpClient, DeviceCodeURL, form, &response); err != nil {
		var statusErr *oauthHTTPError
		if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusUnauthorized {
			return DeviceAuthorization{}, ErrInvalidClient
		}
		return DeviceAuthorization{}, err
	}
	if response.Error != "" {
		return DeviceAuthorization{}, oauthError(response.Error, time.Duration(max(response.Interval, 1))*time.Second)
	}
	verification, err := url.Parse(response.VerificationURI)
	if err != nil || verification.Scheme != "https" || verification.Host != "github.com" || verification.Path != "/login/device" {
		return DeviceAuthorization{}, ErrInvalidDeviceResponse
	}
	if len(response.DeviceCode) == 0 || len(response.DeviceCode) > 2048 || len(response.UserCode) == 0 || len(response.UserCode) > 64 ||
		response.ExpiresIn <= 0 || response.ExpiresIn > 3600 || response.Interval <= 0 || response.Interval > 300 {
		return DeviceAuthorization{}, ErrInvalidDeviceResponse
	}
	return DeviceAuthorization{
		DeviceCode: response.DeviceCode, UserCode: response.UserCode,
		VerificationURI: DeviceVerificationURL, ExpiresIn: time.Duration(response.ExpiresIn) * time.Second,
		Interval: time.Duration(response.Interval) * time.Second,
	}, nil
}

// PollDeviceAuthorization performs one poll. Callers must wait Interval before
// polling and use PendingError.Interval after pending or slow_down responses.
func PollDeviceAuthorization(ctx context.Context, clientID string, device DeviceAuthorization, httpClient *http.Client, now time.Time) (Credentials, time.Duration, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(device.DeviceCode) == "" {
		return Credentials{}, device.Interval, ErrInvalidClient
	}
	form := url.Values{
		"client_id":   {strings.TrimSpace(clientID)},
		"device_code": {device.DeviceCode},
		"grant_type":  {DeviceGrantType},
	}
	var response oauthResponse
	if err := postOAuthJSON(ctx, httpClient, AccessTokenURL, form, &response); err != nil {
		var statusErr *oauthHTTPError
		if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusUnauthorized {
			return Credentials{}, device.Interval, ErrInvalidClient
		}
		return Credentials{}, device.Interval, err
	}
	if response.Error != "" {
		var pending *PendingError
		pollErr := oauthError(response.Error, device.Interval)
		if errors.As(pollErr, &pending) {
			return Credentials{}, pending.Interval, pending
		}
		return Credentials{}, device.Interval, pollErr
	}
	if !validOAuthToken(response.AccessToken) || !strings.EqualFold(response.TokenType, "bearer") {
		return Credentials{}, device.Interval, ErrInvalidDeviceResponse
	}
	credentials := Credentials{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken}
	if response.ExpiresIn > 0 {
		if response.ExpiresIn > 365*24*60*60 || !validOAuthToken(response.RefreshToken) ||
			response.RefreshTokenExpiresIn <= 0 || response.RefreshTokenExpiresIn > 365*24*60*60 {
			return Credentials{}, device.Interval, ErrInvalidDeviceResponse
		}
		credentials.AccessTokenExpiresAt = now.Add(time.Duration(response.ExpiresIn) * time.Second)
		credentials.RefreshTokenExpiresAt = now.Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second)
	}
	if response.RefreshToken != "" && !validOAuthToken(response.RefreshToken) {
		return Credentials{}, device.Interval, ErrInvalidDeviceResponse
	}
	return credentials, device.Interval, nil
}

func oauthError(code string, interval time.Duration) error {
	switch code {
	case "authorization_pending":
		return &PendingError{Interval: interval}
	case "slow_down":
		return &PendingError{Interval: interval + 5*time.Second, SlowDown: true}
	case "access_denied":
		return ErrAuthorizationDenied
	case "expired_token":
		return ErrDeviceCodeExpired
	case "token_expired":
		return ErrDeviceCodeExpired
	case "device_flow_disabled", "unsupported_grant_type":
		return ErrDeviceFlowDisabled
	case "invalid_client", "incorrect_client_credentials", "bad_verification_code", "incorrect_device_code":
		return ErrInvalidClient
	default:
		return fmt.Errorf("GitHub authorization failed (%s)", safeCode(code))
	}
}

func postOAuthJSON(ctx context.Context, httpClient *http.Client, endpoint string, form url.Values, target *oauthResponse) error {
	client := noRedirectClient(httpClient, 15*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("could not prepare GitHub authorization request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AETHER-Dots")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not reach GitHub authorization service")
	}
	defer response.Body.Close()
	body, err := readBounded(response.Body, maxOAuthResponseBytes)
	if err != nil {
		return errors.New("GitHub authorization response was too large or unreadable")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if json.Unmarshal(body, target) == nil && target.Error != "" {
			return nil
		}
		return &oauthHTTPError{statusCode: response.StatusCode}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("GitHub returned an invalid authorization response")
	}
	return nil
}

// Service manages credential refresh and authenticated GitHub API access.
// It serializes refresh-token rotation and cancels every in-flight request on
// disconnect.
type Service struct {
	gh       *ghRunner // non-nil: read through the GitHub CLI instead of OAuth
	clientID string
	vault    CredentialStore
	http     *http.Client
	now      func() time.Time

	mu                sync.Mutex
	refreshMu         sync.Mutex
	disconnected      bool
	authorizing       bool
	sessionGeneration uint64
	authCancel        context.CancelFunc
	nextOperation     uint64
	operations        map[uint64]context.CancelFunc
}

func NewService(clientID string, vault CredentialStore, httpClient *http.Client) *Service {
	return &Service{
		clientID: strings.TrimSpace(clientID), vault: vault, http: httpClient,
		now: time.Now, operations: make(map[uint64]context.CancelFunc),
	}
}

func (s *Service) ClientConfigured() bool {
	if s != nil && s.gh != nil {
		return true
	}
	return s != nil && s.clientID != "" && len(s.clientID) <= 256 && !strings.ContainsAny(s.clientID, "\r\n\x00")
}

func (s *Service) Connected(ctx context.Context) bool {
	if s == nil || (s.vault == nil && s.gh == nil) {
		return false
	}
	s.mu.Lock()
	blocked := s.disconnected
	s.mu.Unlock()
	if blocked {
		return false
	}
	if s.gh != nil {
		return s.gh.authenticated(ctx)
	}
	credentials, err := s.vault.Load(ctx)
	return err == nil && credentials.AccessToken != ""
}

// AuthorizeDevice owns one cancellable Device Flow. onDevice runs once the
// verification code is ready, then the function polls at GitHub's cadence.
func (s *Service) AuthorizeDevice(ctx context.Context, onDevice func(DeviceAuthorization)) error {
	if s != nil && s.gh != nil {
		// gh owns the login. "Connecting" only confirms it is signed in, so no
		// verification code is shown and onDevice is never called.
		s.gh.forgetAuth()
		if !s.gh.authenticated(ctx) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrCLINotAuthenticated
		}
		s.mu.Lock()
		s.disconnected = false
		s.sessionGeneration++
		s.mu.Unlock()
		return nil
	}
	if s == nil || s.vault == nil || !s.ClientConfigured() {
		return ErrInvalidClient
	}
	s.refreshMu.Lock()
	s.mu.Lock()
	if s.authorizing {
		s.mu.Unlock()
		s.refreshMu.Unlock()
		return ErrAuthorizationInProgress
	}
	s.authorizing = true
	s.disconnected = false
	s.sessionGeneration++
	authCtx, authCancel := context.WithCancel(ctx)
	s.authCancel = authCancel
	s.mu.Unlock()
	s.refreshMu.Unlock()
	defer func() {
		authCancel()
		s.mu.Lock()
		s.authorizing = false
		s.authCancel = nil
		s.mu.Unlock()
	}()
	device, err := BeginDeviceAuthorization(authCtx, s.clientID, s.http)
	if err != nil {
		return err
	}
	if onDevice != nil {
		onDevice(device)
	}
	deadline := s.now().Add(device.ExpiresIn)
	interval := device.Interval
	for {
		if err := waitFor(authCtx, interval); err != nil {
			return err
		}
		if !s.now().Before(deadline) {
			return ErrDeviceCodeExpired
		}
		device.Interval = interval
		credentials, nextInterval, err := PollDeviceAuthorization(authCtx, s.clientID, device, s.http, s.now())
		if err != nil {
			var pending *PendingError
			if errors.As(err, &pending) {
				interval = nextInterval
				continue
			}
			return err
		}
		s.mu.Lock()
		if s.disconnected || authCtx.Err() != nil {
			s.mu.Unlock()
			return ErrNotConnected
		}
		err = s.vault.Save(authCtx, credentials)
		if err == nil {
			s.disconnected = false
		}
		s.mu.Unlock()
		if err != nil {
			return fmt.Errorf("GitHub authorized, but secure credential storage failed: %s", safeCredentialStoreError(err))
		}
		return nil
	}
}

func (s *Service) Disconnect(ctx context.Context) error {
	if s != nil && s.gh != nil {
		// AETHER never owned the gh login, so only stop using it.
		s.CancelActiveRequests()
		return nil
	}
	if s == nil || s.vault == nil {
		return nil
	}
	s.CancelActiveRequests()
	s.refreshMu.Lock()
	// A connection attempt may have started between the first invalidation and
	// this lock. Invalidate again while new authorizations are serialized here.
	s.CancelActiveRequests()
	if err := s.vault.Delete(ctx); err != nil {
		s.refreshMu.Unlock()
		return fmt.Errorf("could not remove GitHub credentials from the system credential store: %s", safeCredentialStoreError(err))
	}
	s.refreshMu.Unlock()
	return nil
}

// CancelActiveRequests invalidates the in-memory session and stops active
// authorization or API work before credential deletion completes.
func (s *Service) CancelActiveRequests() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.disconnected = true
	s.sessionGeneration++
	if s.authCancel != nil {
		s.authCancel()
	}
	for _, cancel := range s.operations {
		cancel()
	}
	s.operations = make(map[uint64]context.CancelFunc)
	s.mu.Unlock()
}

func waitFor(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) withClient(ctx context.Context, operation func(context.Context, *APIClient) error) error {
	if s == nil || (s.vault == nil && s.gh == nil) {
		return ErrNotConnected
	}
	s.mu.Lock()
	if s.disconnected {
		s.mu.Unlock()
		return ErrNotConnected
	}
	s.nextOperation++
	id := s.nextOperation
	sessionGeneration := s.sessionGeneration
	opCtx, cancel := context.WithCancel(ctx)
	if s.operations == nil {
		s.operations = make(map[uint64]context.CancelFunc)
	}
	s.operations[id] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.operations, id)
		s.mu.Unlock()
	}()
	if s.gh != nil {
		if !s.gh.authenticated(opCtx) {
			if opCtx.Err() != nil {
				return opCtx.Err()
			}
			return ErrCLINotAuthenticated
		}
		return operation(opCtx, &APIClient{gh: s.gh})
	}
	credentials, err := s.currentCredentials(opCtx)
	if err != nil {
		if errors.Is(err, ErrSessionExpired) {
			s.invalidateSession(sessionGeneration)
		}
		return err
	}
	if err := opCtx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	disconnected := s.disconnected
	s.mu.Unlock()
	if disconnected {
		return ErrNotConnected
	}
	err = operation(opCtx, NewAPIClient(credentials.AccessToken, s.http))
	if errors.Is(err, ErrSessionExpired) {
		s.invalidateSession(sessionGeneration)
	}
	return err
}

func (s *Service) invalidateSession(expectedGeneration uint64) {
	s.mu.Lock()
	if s.disconnected || s.sessionGeneration != expectedGeneration {
		s.mu.Unlock()
		return
	}
	s.disconnected = true
	s.sessionGeneration++
	invalidatedGeneration := s.sessionGeneration
	for _, cancel := range s.operations {
		cancel()
	}
	s.operations = make(map[uint64]context.CancelFunc)
	s.mu.Unlock()
	s.refreshMu.Lock()
	s.mu.Lock()
	stillExpired := s.disconnected && s.sessionGeneration == invalidatedGeneration
	s.mu.Unlock()
	if !stillExpired {
		s.refreshMu.Unlock()
		return
	}
	_ = s.vault.Delete(context.Background())
	s.refreshMu.Unlock()
}

func (s *Service) currentCredentials(ctx context.Context) (Credentials, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.mu.Lock()
	disconnected := s.disconnected
	s.mu.Unlock()
	if disconnected {
		return Credentials{}, ErrNotConnected
	}
	credentials, err := s.vault.Load(ctx)
	if errors.Is(err, ErrCredentialsCorrupt) {
		return Credentials{}, err
	}
	if err != nil {
		return Credentials{}, ErrNotConnected
	}
	if strings.TrimSpace(credentials.AccessToken) == "" {
		return Credentials{}, ErrNotConnected
	}
	if credentials.AccessTokenExpiresAt.IsZero() || credentials.AccessTokenExpiresAt.After(s.now().Add(refreshBeforeExpiry)) {
		return credentials, nil
	}
	if strings.TrimSpace(credentials.RefreshToken) == "" || (!credentials.RefreshTokenExpiresAt.IsZero() && !credentials.RefreshTokenExpiresAt.After(s.now())) {
		_ = s.vault.Delete(ctx)
		return Credentials{}, ErrSessionExpired
	}
	rotated, err := s.refresh(ctx, credentials.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrSessionExpired) {
			_ = s.vault.Delete(ctx)
		}
		return Credentials{}, err
	}
	s.mu.Lock()
	if s.disconnected || ctx.Err() != nil {
		s.mu.Unlock()
		return Credentials{}, ErrNotConnected
	}
	err = s.vault.Save(ctx, rotated)
	s.mu.Unlock()
	if err != nil {
		return Credentials{}, fmt.Errorf("could not save rotated GitHub credentials securely: %s", safeCredentialStoreError(err))
	}
	return rotated, nil
}

func (s *Service) refresh(ctx context.Context, refreshToken string) (Credentials, error) {
	if !s.ClientConfigured() {
		return Credentials{}, ErrInvalidClient
	}
	form := url.Values{
		"client_id":     {s.clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	var response oauthResponse
	if err := postOAuthJSON(ctx, s.http, AccessTokenURL, form, &response); err != nil {
		var statusErr *oauthHTTPError
		if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusUnauthorized {
			return Credentials{}, ErrInvalidClient
		}
		return Credentials{}, err
	}
	if response.Error != "" {
		switch response.Error {
		case "bad_refresh_token", "expired_token", "access_denied", "invalid_grant":
			return Credentials{}, ErrSessionExpired
		default:
			return Credentials{}, fmt.Errorf("GitHub credential refresh failed (%s)", safeCode(response.Error))
		}
	}
	if !validOAuthToken(response.AccessToken) || !validOAuthToken(response.RefreshToken) || response.ExpiresIn <= 0 || response.RefreshTokenExpiresIn <= 0 ||
		response.ExpiresIn > 365*24*60*60 || response.RefreshTokenExpiresIn > 365*24*60*60 {
		return Credentials{}, ErrSessionExpired
	}
	now := s.now()
	return Credentials{
		AccessToken: response.AccessToken, RefreshToken: response.RefreshToken,
		AccessTokenExpiresAt:  now.Add(time.Duration(response.ExpiresIn) * time.Second),
		RefreshTokenExpiresAt: now.Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second),
	}, nil
}

func safeCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return "unknown"
		}
	}
	return value
}

func validOAuthToken(value string) bool {
	if value == "" || len(value) > 2048 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func safeCredentialStoreError(err error) string {
	if err == nil {
		return "credential store unavailable"
	}
	message := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, err.Error())
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 256 {
		message = message[:256]
	}
	if message == "" {
		return "credential store unavailable"
	}
	return message
}

func readBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("response too large")
	}
	return data, nil
}

func noRedirectClient(client *http.Client, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	var transport http.RoundTripper
	if client != nil {
		transport = client.Transport
		if client.Timeout > 0 && client.Timeout < timeout {
			timeout = client.Timeout
		}
	}
	return &http.Client{
		Transport: transport, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
