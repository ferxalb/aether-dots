package license

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testOrg        = "11111111-2222-3333-4444-555555555555"
	testActivation = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	testKey        = "AETHER-1234-ABCD"
)

var day = 24 * time.Hour

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func TestEvaluate(t *testing.T) {
	now := date("2027-06-01")
	annual := func(expires string, validated time.Time) *State {
		return &State{Key: testKey, ActivatedAt: date("2026-06-01"), ExpiresAt: ptr(date(expires)), LastValidatedAt: validated}
	}
	sub := func(activated string, validated time.Time) *State {
		return &State{Key: testKey, ActivatedAt: date(activated), LastValidatedAt: validated}
	}
	cases := []struct {
		name   string
		state  *State
		build  time.Time
		tier   Tier
		reason Reason
	}{
		{"no license", nil, date("2027-01-01"), Free, ReasonNoLicense},
		{"annual active", annual("2027-12-01", now.Add(-day)), date("2027-05-01"), Pro, ReasonActive},
		{"active ignores build date", annual("2027-12-01", now.Add(-day)), date("2027-05-30"), Pro, ReasonActive},
		{"annual offline past grace keeps paid builds", annual("2027-12-01", now.Add(-30*day)), date("2027-05-01"), Pro, ReasonFallback},
		{"annual expired, older build keeps Pro", annual("2027-05-01", date("2027-04-30")), date("2027-04-15"), Pro, ReasonFallback},
		{"annual expired, newer build is Free", annual("2027-05-01", date("2027-04-30")), date("2027-05-15"), Free, ReasonUnpaidBuild},
		{"dev build gets no fallback", annual("2027-05-01", date("2027-04-30")), time.Time{}, Free, ReasonExpired},
		{"subscription active", sub("2027-05-01", now.Add(-2*day)), date("2027-05-30"), Pro, ReasonActive},
		{"short subscription offline gets no fallback", sub("2027-03-01", now.Add(-20*day)), date("2027-04-01"), Free, ReasonNeedsCheck},
		{"year-long subscription keeps covered builds", sub("2026-04-01", now.Add(-20*day)), date("2027-04-01"), Pro, ReasonFallback},
		{"revoked short subscription is Free", func() *State { s := sub("2027-03-01", now.Add(-day)); s.Revoked = true; return s }(), date("2027-04-01"), Free, ReasonRevoked},
		{"refunded annual key gets no fallback", func() *State { s := annual("2027-12-01", date("2027-05-01")); s.Revoked = true; return s }(), date("2027-04-01"), Free, ReasonRevoked},
		{"revoked year-long subscription keeps builds before its last check", func() *State { s := sub("2026-01-01", date("2027-03-01")); s.Revoked = true; return s }(), date("2027-02-01"), Pro, ReasonFallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.state, tc.build, now)
			if got.Tier != tc.tier || got.Reason != tc.reason {
				t.Fatalf("Evaluate = %s (%s), want %s (%s)", got.Tier, got.Reason, tc.tier, tc.reason)
			}
		})
	}
}

func TestEvaluateActiveUntilIsEarlierOfExpiryAndGrace(t *testing.T) {
	now := date("2027-06-01")
	s := &State{Key: testKey, ExpiresAt: ptr(date("2027-06-05")), LastValidatedAt: now}
	if got := Evaluate(s, time.Time{}, now); !got.Until.Equal(date("2027-06-05")) {
		t.Fatalf("Until = %v, want the expiry", got.Until)
	}
	s.ExpiresAt = nil
	if got := Evaluate(s, time.Time{}, now); !got.Until.Equal(now.Add(OfflineGrace)) {
		t.Fatalf("Until = %v, want the end of the offline grace", got.Until)
	}
}

func TestBuildConfig(t *testing.T) {
	defer func(o, e, b string) { OrganizationID, Environment, BuildDate = o, e, b }(OrganizationID, Environment, BuildDate)
	OrganizationID, Environment, BuildDate = "", "", ""
	if _, err := BuildConfig(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("empty organization: err = %v, want ErrDisabled", err)
	}
	OrganizationID = "not-a-uuid"
	if _, err := BuildConfig(); err == nil {
		t.Fatal("accepted a malformed organization ID")
	}
	OrganizationID, Environment, BuildDate = testOrg, "sandbox", "2027-01-02T03:04:05Z"
	cfg, err := BuildConfig()
	if err != nil || cfg.BaseURL != sandboxBaseURL || !cfg.BuildDate.Equal(time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("BuildConfig = %+v, %v", cfg, err)
	}
	Environment = ""
	if cfg, _ := BuildConfig(); cfg.BaseURL != productionBaseURL {
		t.Fatalf("default base URL = %q", cfg.BaseURL)
	}
	Environment = "https://evil.example"
	if _, err := BuildConfig(); err == nil {
		t.Fatal("accepted an arbitrary environment")
	}
}

func TestNormalizeKey(t *testing.T) {
	if k, err := NormalizeKey("  " + testKey + "\n"); err != nil || k != testKey {
		t.Fatalf("NormalizeKey = %q, %v", k, err)
	}
	for _, bad := range []string{"", "has space", "tab\tkey", "ñ", strings.Repeat("a", 257)} {
		if _, err := NormalizeKey(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

// fakePolar records requests and answers like Polar's customer-portal API.
type fakePolar struct {
	mu          sync.Mutex
	requests    []string
	bodies      []map[string]any
	validate    func(body map[string]any) (int, any)
	activateErr int
}

func (f *fakePolar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	reply := func(code int, v any) {
		w.WriteHeader(code)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	switch r.URL.Path {
	case "/v1/customer-portal/license-keys/activate":
		if f.activateErr != 0 {
			reply(f.activateErr, map[string]string{"error": "NotPermitted"})
			return
		}
		reply(200, map[string]any{"id": testActivation, "license_key_id": testOrg})
	case "/v1/customer-portal/license-keys/validate":
		reply(f.validate(body))
	case "/v1/customer-portal/license-keys/deactivate":
		reply(204, nil)
	default:
		reply(404, nil)
	}
}

func granted(expires *time.Time) func(map[string]any) (int, any) {
	return func(body map[string]any) (int, any) {
		return 200, map[string]any{
			"organization_id": testOrg, "key": body["key"], "display_key": "****-ABCD",
			"status": "granted", "expires_at": expires,
			"activation": map[string]any{"id": body["activation_id"]},
		}
	}
}

func newTestClient(t *testing.T, f *fakePolar, now time.Time) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := NewClient(Config{OrganizationID: testOrg, BaseURL: srv.URL})
	c.now = func() time.Time { return now }
	return c
}

func TestActivateValidatesAndSendsOnlyKeyOrgLabel(t *testing.T) {
	now := date("2027-01-10")
	f := &fakePolar{validate: granted(ptr(date("2028-01-10")))}
	c := newTestClient(t, f, now)
	s, err := c.Activate(context.Background(), " "+testKey+" ")
	if err != nil {
		t.Fatal(err)
	}
	if s.ActivationID != testActivation || s.Key != testKey || s.DisplayKey != "****-ABCD" ||
		!s.LastValidatedAt.Equal(now) || !s.ActivatedAt.Equal(now) || s.ExpiresAt == nil || !s.ExpiresAt.Equal(date("2028-01-10")) {
		t.Fatalf("state = %+v", s)
	}
	if strings.Join(f.requests, ",") != "POST /v1/customer-portal/license-keys/activate,POST /v1/customer-portal/license-keys/validate" {
		t.Fatalf("requests = %v", f.requests)
	}
	act := f.bodies[0]
	if len(act) != 3 || act["key"] != testKey || act["organization_id"] != testOrg || act["label"] != "AETHER "+runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("activate body = %v", act)
	}
	if got := Evaluate(s, time.Time{}, now); got.Tier != Pro {
		t.Fatalf("fresh activation tier = %s", got.Tier)
	}
}

func TestActivateLimitReached(t *testing.T) {
	f := &fakePolar{activateErr: 403}
	c := newTestClient(t, f, date("2027-01-10"))
	if _, err := c.Activate(context.Background(), testKey); !errors.Is(err, ErrNotPermitted) {
		t.Fatalf("err = %v, want ErrNotPermitted", err)
	}
}

func TestActivateReleasesSlotWhenValidationFails(t *testing.T) {
	f := &fakePolar{validate: func(map[string]any) (int, any) { return 404, nil }}
	c := newTestClient(t, f, date("2027-01-10"))
	if _, err := c.Activate(context.Background(), testKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if last := f.requests[len(f.requests)-1]; last != "POST /v1/customer-portal/license-keys/deactivate" {
		t.Fatalf("last request = %s, want deactivate", last)
	}
}

func TestValidateOutcomes(t *testing.T) {
	now := date("2027-03-01")
	base := func() *State {
		return &State{Version: stateVersion, Key: testKey, ActivationID: testActivation, ActivatedAt: date("2027-01-01"), LastValidatedAt: date("2027-02-01")}
	}

	t.Run("revoked subscription", func(t *testing.T) {
		c := newTestClient(t, &fakePolar{validate: func(map[string]any) (int, any) { return 404, nil }}, now)
		s := base()
		if err := c.Validate(context.Background(), s); !errors.Is(err, ErrNotFound) || !s.Revoked {
			t.Fatalf("err = %v revoked = %v", err, s.Revoked)
		}
	})
	t.Run("naturally expired annual key is not revoked", func(t *testing.T) {
		c := newTestClient(t, &fakePolar{validate: func(map[string]any) (int, any) { return 404, nil }}, now)
		s := base()
		s.ExpiresAt = ptr(date("2027-02-15"))
		if err := c.Validate(context.Background(), s); !errors.Is(err, ErrNotFound) || s.Revoked {
			t.Fatalf("err = %v revoked = %v", err, s.Revoked)
		}
		if !s.PaidThrough().Equal(date("2027-02-15")) {
			t.Fatalf("PaidThrough = %v", s.PaidThrough())
		}
	})
	t.Run("server error leaves state unchanged", func(t *testing.T) {
		c := newTestClient(t, &fakePolar{validate: func(map[string]any) (int, any) { return 503, nil }}, now)
		s := base()
		before := *s
		if err := c.Validate(context.Background(), s); !errors.Is(err, ErrUnavailable) || s.Revoked || !s.LastValidatedAt.Equal(before.LastValidatedAt) {
			t.Fatalf("err = %v state = %+v", err, s)
		}
	})
	t.Run("response for another organization is rejected", func(t *testing.T) {
		c := newTestClient(t, &fakePolar{validate: func(body map[string]any) (int, any) {
			_, v := granted(nil)(body)
			v.(map[string]any)["organization_id"] = "99999999-2222-3333-4444-555555555555"
			return 200, v
		}}, now)
		s := base()
		if err := c.Validate(context.Background(), s); !errors.Is(err, ErrMismatch) || !s.LastValidatedAt.Equal(date("2027-02-01")) {
			t.Fatalf("err = %v state = %+v", err, s)
		}
	})
	t.Run("granted renews and clears revoked", func(t *testing.T) {
		c := newTestClient(t, &fakePolar{validate: granted(nil)}, now)
		s := base()
		s.Revoked = true
		if err := c.Validate(context.Background(), s); err != nil || s.Revoked || !s.LastValidatedAt.Equal(now) || s.ExpiresAt != nil {
			t.Fatalf("err = %v state = %+v", err, s)
		}
	})
}

func TestClientRefusesRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect was followed to %s", r.URL)
	}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := NewClient(Config{OrganizationID: testOrg, BaseURL: srv.URL})
	if _, err := c.Activate(context.Background(), testKey); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestStateRoundTripAndPermissions(t *testing.T) {
	dir := t.TempDir()
	if s, err := Load(dir); s != nil || err != nil {
		t.Fatalf("missing file: %v, %v", s, err)
	}
	want := &State{Version: stateVersion, Key: testKey, ActivationID: testActivation, ExpiresAt: ptr(date("2028-01-01")), LastValidatedAt: date("2027-01-01")}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.Key != want.Key || !got.ExpiresAt.Equal(*want.ExpiresAt) {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	path := filepath.Join(dir, stateFile)
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("loaded a world-readable license file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("followed a symlinked license file")
	}
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir); err != nil {
		t.Fatalf("Remove of a missing file: %v", err)
	}
}
