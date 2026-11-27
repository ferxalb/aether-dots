package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/license"
)

type fakeLicenseClient struct {
	activatedKey string
	validateErr  error
	deactivated  int
}

func (f *fakeLicenseClient) Activate(_ context.Context, key string) (*license.State, error) {
	key, err := license.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	f.activatedKey = key
	exp := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	return &license.State{Version: 1, Key: key, DisplayKey: "****-ABCD", ActivationID: "act", ActivatedAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), ExpiresAt: &exp, LastValidatedAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeLicenseClient) Validate(_ context.Context, s *license.State) error {
	if f.validateErr != nil {
		if errors.Is(f.validateErr, license.ErrNotFound) {
			s.Revoked = true
		}
		return f.validateErr
	}
	s.LastValidatedAt = time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	return nil
}

func (f *fakeLicenseClient) Deactivate(context.Context, *license.State) error {
	f.deactivated++
	return nil
}

func TestLicenseCommandFlow(t *testing.T) {
	dir := t.TempDir()
	cfg := license.Config{OrganizationID: "11111111-2222-3333-4444-555555555555", BuildDate: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)}
	client := &fakeLicenseClient{}
	now := func() time.Time { return time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC) }
	run := func(cmd, stdin string) (string, error) {
		var out bytes.Buffer
		err := runLicenseWith(cfg, client, dir, cmd, strings.NewReader(stdin), &out, now)
		return out.String(), err
	}

	if out, err := run("status", ""); err != nil || !strings.Contains(out, "Tier: free (no license)") {
		t.Fatalf("status before activation: %q, %v", out, err)
	}
	out, err := run("activate", "  AETHER-1234-ABCD  \nignored second line")
	if err != nil || client.activatedKey != "AETHER-1234-ABCD" || !strings.Contains(out, "Tier: pro (active)") || !strings.Contains(out, "Paid through: 2028-01-01") {
		t.Fatalf("activate: %q, %v, key %q", out, err, client.activatedKey)
	}
	if strings.Contains(out, "AETHER-1234-ABCD") {
		t.Fatalf("activate printed the full key: %q", out)
	}
	if _, err := run("activate", "OTHER-KEY\n"); err == nil {
		t.Fatal("activated a second key over an active one")
	}

	client.validateErr = license.ErrUnavailable
	if _, err := run("check", ""); !errors.Is(err, license.ErrUnavailable) {
		t.Fatalf("check offline: %v", err)
	}
	if s, _ := license.Load(dir); s.Revoked || !s.LastValidatedAt.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("offline check changed the saved state: %+v", s)
	}

	client.validateErr = license.ErrNotFound
	if _, err := run("check", ""); !errors.Is(err, license.ErrNotFound) {
		t.Fatalf("check revoked: %v", err)
	}
	if s, _ := license.Load(dir); !s.Revoked {
		t.Fatal("revocation was not saved")
	}

	if out, err := run("deactivate", ""); err != nil || client.deactivated != 1 || !strings.Contains(out, "released") {
		t.Fatalf("deactivate: %q, %v", out, err)
	}
	if s, _ := license.Load(dir); s != nil {
		t.Fatalf("license file left after deactivate: %+v", s)
	}
}
