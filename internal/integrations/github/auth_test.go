package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type githubRoundTripFunc func(*http.Request) (*http.Response, error)

func (f githubRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func oauthHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func oauthTestClient(roundTrip githubRoundTripFunc) *http.Client {
	return &http.Client{Transport: roundTrip}
}

func TestBeginDeviceAuthorizationUsesGitHubDeviceFlow(t *testing.T) {
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != DeviceCodeURL {
			t.Fatalf("unexpected device authorization request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("device flow sent an authorization header")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		if form.Get("client_id") != "public-client-id" || form.Get("scope") != "offline_access" {
			t.Fatalf("device flow form=%v", form)
		}
		return oauthHTTPResponse(http.StatusOK, `{"device_code":"device-secret","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":600,"interval":5}`), nil
	})
	device, err := BeginDeviceAuthorization(context.Background(), " public-client-id ", client)
	if err != nil {
		t.Fatal(err)
	}
	if device.DeviceCode != "device-secret" || device.UserCode != "ABCD-EFGH" || device.Interval != 5*time.Second || device.ExpiresIn != 10*time.Minute {
		t.Fatalf("device authorization parsed incorrectly: %#v", device)
	}
}

func TestBeginDeviceAuthorizationRejectsUntrustedVerificationURL(t *testing.T) {
	client := oauthTestClient(func(*http.Request) (*http.Response, error) {
		return oauthHTTPResponse(http.StatusOK, `{"device_code":"code","user_code":"ABCD","verification_uri":"https://example.com/login/device","expires_in":600,"interval":5}`), nil
	})
	if _, err := BeginDeviceAuthorization(context.Background(), "client", client); !errors.Is(err, ErrInvalidDeviceResponse) {
		t.Fatalf("unexpected verification host error=%v", err)
	}
}

func TestDeviceFlowMapsUnauthorizedHTTPAndJSONCredentialErrors(t *testing.T) {
	unauthorized := oauthTestClient(func(*http.Request) (*http.Response, error) {
		return oauthHTTPResponse(http.StatusUnauthorized, `{"error":"invalid_client"}`), nil
	})
	if _, err := BeginDeviceAuthorization(context.Background(), "client", unauthorized); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("unauthorized device request error=%v", err)
	}
	badCode := oauthTestClient(func(*http.Request) (*http.Response, error) {
		return oauthHTTPResponse(http.StatusBadRequest, `{"error":"incorrect_device_code"}`), nil
	})
	_, _, err := PollDeviceAuthorization(context.Background(), "client", DeviceAuthorization{DeviceCode: "device", Interval: time.Second}, badCode, time.Now())
	if !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("invalid device credentials error=%v", err)
	}
}

func TestDeviceAuthorizationHonorsNetworkTimeoutAndSuppressesTransportDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	_, err := BeginDeviceAuthorization(ctx, "public-client", client)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("device authorization timeout error=%v", err)
	}

	client = oauthTestClient(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport diagnostic included ghu_sensitive_detail")
	})
	_, err = BeginDeviceAuthorization(context.Background(), "public-client", client)
	if err == nil || strings.Contains(err.Error(), "ghu_sensitive_detail") || !strings.Contains(err.Error(), "could not reach GitHub") {
		t.Fatalf("transport error was not sanitized: %v", err)
	}
}

func TestPollDeviceAuthorizationStatesAndTokenExpiry(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       string
		wantErr    error
		wantSlow   bool
		wantPeriod time.Duration
	}{
		{name: "pending", body: `{"error":"authorization_pending"}`, wantPeriod: 3 * time.Second},
		{name: "slow down", body: `{"error":"slow_down"}`, wantSlow: true, wantPeriod: 8 * time.Second},
		{name: "denied", body: `{"error":"access_denied"}`, wantErr: ErrAuthorizationDenied},
		{name: "expired", body: `{"error":"expired_token"}`, wantErr: ErrDeviceCodeExpired},
		{name: "documented expired code alias", body: `{"error":"token_expired"}`, wantErr: ErrDeviceCodeExpired},
		{name: "disabled device flow", body: `{"error":"device_flow_disabled"}`, wantErr: ErrDeviceFlowDisabled},
		{name: "invalid client", body: `{"error":"invalid_client"}`, wantErr: ErrInvalidClient},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != AccessTokenURL {
					t.Fatalf("unexpected token URL: %s", request.URL)
				}
				return oauthHTTPResponse(http.StatusOK, test.body), nil
			})
			device := DeviceAuthorization{DeviceCode: "device", Interval: 3 * time.Second}
			credentials, next, err := PollDeviceAuthorization(context.Background(), "client", device, client, time.Now())
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error=%v, want %v", err, test.wantErr)
				}
				return
			}
			var pending *PendingError
			if !errors.As(err, &pending) || pending.SlowDown != test.wantSlow || next != test.wantPeriod {
				t.Fatalf("pending=%#v next=%s err=%v", pending, next, err)
			}
			if credentials.AccessToken != "" {
				t.Fatalf("pending response unexpectedly produced credentials: %#v", credentials)
			}
		})
	}

	now := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != DeviceGrantType || form.Get("device_code") != "device" {
			t.Fatalf("unexpected device token form: %v", form)
		}
		return oauthHTTPResponse(http.StatusOK, `{"access_token":"ghu_access","refresh_token":"ghr_refresh","token_type":"bearer","expires_in":3600,"refresh_token_expires_in":86400}`), nil
	})
	credentials, _, err := PollDeviceAuthorization(context.Background(), "client", DeviceAuthorization{DeviceCode: "device", Interval: time.Second}, client, now)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessTokenExpiresAt != now.Add(time.Hour) || credentials.RefreshTokenExpiresAt != now.Add(24*time.Hour) {
		t.Fatalf("token expiry metadata was not parsed: %#v", credentials)
	}
}

type memoryCredentialStore struct {
	mu          sync.Mutex
	credentials Credentials
	deleted     bool
	saves       int
}

func (s *memoryCredentialStore) Load(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleted || s.credentials.AccessToken == "" {
		return Credentials{}, ErrNotConnected
	}
	return s.credentials, nil
}

func (s *memoryCredentialStore) Save(ctx context.Context, credentials Credentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.credentials = credentials
	s.deleted = false
	s.saves++
	s.mu.Unlock()
	return nil
}

func (s *memoryCredentialStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.credentials = Credentials{}
	s.deleted = true
	s.mu.Unlock()
	return nil
}

func TestAuthorizeDeviceSuccessAndCancellation(t *testing.T) {
	var polls int
	var mu sync.Mutex
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case DeviceCodeURL:
			return oauthHTTPResponse(http.StatusOK, `{"device_code":"device","user_code":"ABCD","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`), nil
		case AccessTokenURL:
			mu.Lock()
			polls++
			mu.Unlock()
			return oauthHTTPResponse(http.StatusOK, `{"access_token":"ghu_access","refresh_token":"ghr_refresh","token_type":"bearer","expires_in":3600,"refresh_token_expires_in":86400}`), nil
		default:
			t.Fatalf("unexpected OAuth endpoint: %s", request.URL)
			return nil, errors.New("unexpected endpoint")
		}
	})
	vault := &memoryCredentialStore{}
	service := NewService("public-client", vault, client)
	var shown DeviceAuthorization
	if err := service.AuthorizeDevice(context.Background(), func(device DeviceAuthorization) { shown = device }); err != nil {
		t.Fatal(err)
	}
	if shown.UserCode != "ABCD" || !service.Connected(context.Background()) {
		t.Fatalf("authorization did not finish with a stored token: device=%#v", shown)
	}
	vault.mu.Lock()
	stored := vault.credentials
	vault.mu.Unlock()
	if stored.AccessToken != "ghu_access" || stored.RefreshToken != "ghr_refresh" || stored.AccessTokenExpiresAt.IsZero() {
		t.Fatalf("authorization did not store rotated token metadata: %#v", stored)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelService := NewService("public-client", &memoryCredentialStore{}, oauthTestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == DeviceCodeURL {
			return oauthHTTPResponse(http.StatusOK, `{"device_code":"device","user_code":"ABCD","verification_uri":"https://github.com/login/device","expires_in":600,"interval":300}`), nil
		}
		t.Fatal("canceled authorization polled GitHub")
		return nil, errors.New("unexpected poll")
	}))
	err := cancelService.AuthorizeDevice(ctx, func(DeviceAuthorization) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled device flow error=%v", err)
	}
	if cancelService.Connected(context.Background()) {
		t.Fatal("canceled authorization saved credentials")
	}
}

func TestAuthorizeDevicePreventsDuplicateFlowAndDisconnectDeletesCredentials(t *testing.T) {
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != DeviceCodeURL {
			t.Fatal("duplicate authorization sent an unexpected request")
		}
		return oauthHTTPResponse(http.StatusOK, `{"device_code":"device","user_code":"ABCD","verification_uri":"https://github.com/login/device","expires_in":600,"interval":300}`), nil
	})
	vault := &memoryCredentialStore{}
	service := NewService("public-client", vault, client)
	done := make(chan error, 1)
	go func() {
		done <- service.AuthorizeDevice(ctx, func(DeviceAuthorization) { close(ready) })
	}()
	<-ready
	if err := service.AuthorizeDevice(context.Background(), nil); !errors.Is(err, ErrAuthorizationInProgress) {
		t.Fatalf("second device flow error=%v", err)
	}
	if err := service.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("active authorization after disconnect error=%v", err)
	}
	if service.Connected(context.Background()) {
		t.Fatal("service stayed connected after disconnect")
	}
}

func TestRefreshTokenRotationIsSerializedAndDisconnectRemovesCredentials(t *testing.T) {
	now := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	vault := &memoryCredentialStore{credentials: Credentials{
		AccessToken: "ghu_old", RefreshToken: "ghr_old",
		AccessTokenExpiresAt: now.Add(time.Minute), RefreshTokenExpiresAt: now.Add(24 * time.Hour),
	}}
	var refreshCalls int
	var mu sync.Mutex
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() == AccessTokenURL {
			mu.Lock()
			refreshCalls++
			mu.Unlock()
			body, _ := io.ReadAll(request.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("refresh_token") != "ghr_old" {
				t.Fatalf("refresh did not use stored token: %v", form)
			}
			return oauthHTTPResponse(http.StatusOK, `{"access_token":"ghu_new","refresh_token":"ghr_new","token_type":"bearer","expires_in":3600,"refresh_token_expires_in":86400}`), nil
		}
		if request.URL.Path != "/user" || request.Header.Get("Authorization") != "Bearer ghu_new" {
			t.Fatalf("API request did not use rotated access token: %s %q", request.URL, request.Header.Get("Authorization"))
		}
		return oauthHTTPResponse(http.StatusOK, `{"login":"octocat","html_url":"https://github.com/octocat"}`), nil
	})
	service := NewService("public-client", vault, client)
	service.now = func() time.Time { return now }
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.GetAuthenticatedUser(context.Background())
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	gotRefreshCalls := refreshCalls
	mu.Unlock()
	if gotRefreshCalls != 1 {
		t.Fatalf("concurrent API calls rotated credentials %d times", gotRefreshCalls)
	}
	vault.mu.Lock()
	stored := vault.credentials
	vault.mu.Unlock()
	if stored.AccessToken != "ghu_new" || stored.RefreshToken != "ghr_new" || !stored.AccessTokenExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("rotated credentials were not persisted: %#v", stored)
	}
	if err := service.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.Connected(context.Background()) {
		t.Fatal("credentials remained connected after deletion")
	}
}

func TestDisconnectCancelsActiveAuthenticatedRequest(t *testing.T) {
	vault := &memoryCredentialStore{credentials: Credentials{AccessToken: "ghu_active"}}
	started := make(chan struct{})
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	service := NewService("public-client", vault, client)
	done := make(chan error, 1)
	go func() {
		_, err := service.GetAuthenticatedUser(context.Background())
		done <- err
	}()
	<-started
	if err := service.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight request after disconnect error=%v", err)
	}
	if service.Connected(context.Background()) {
		t.Fatal("credential deletion did not take effect")
	}
}

func TestUnauthorizedAPIResponseInvalidatesExpiredSession(t *testing.T) {
	vault := &memoryCredentialStore{credentials: Credentials{AccessToken: "ghu_expired"}}
	client := oauthTestClient(func(*http.Request) (*http.Response, error) {
		return oauthHTTPResponse(http.StatusUnauthorized, `{"message":"Bad credentials"}`), nil
	})
	service := NewService("public-client", vault, client)
	_, err := service.GetAuthenticatedUser(context.Background())
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("unauthorized API error=%v, want ErrSessionExpired", err)
	}
	if service.Connected(context.Background()) {
		t.Fatal("unauthorized API session remained active")
	}
	if _, err := vault.Load(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("unauthorized API credentials were not removed: %v", err)
	}
}

func TestRefreshUnauthorizedErrorRemovesRotatedSession(t *testing.T) {
	now := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)
	vault := &memoryCredentialStore{credentials: Credentials{
		AccessToken: "ghu_expiring", RefreshToken: "ghr_refresh",
		AccessTokenExpiresAt: now.Add(time.Minute), RefreshTokenExpiresAt: now.Add(time.Hour),
	}}
	client := oauthTestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != AccessTokenURL {
			t.Fatalf("unexpected OAuth endpoint %s", request.URL)
		}
		return oauthHTTPResponse(http.StatusUnauthorized, `{"error":"bad_refresh_token"}`), nil
	})
	service := NewService("public-client", vault, client)
	service.now = func() time.Time { return now }
	_, err := service.GetAuthenticatedUser(context.Background())
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("refresh error=%v, want ErrSessionExpired", err)
	}
	if service.Connected(context.Background()) {
		t.Fatal("failed refresh left the session active")
	}
}
