package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/license"

	"github.com/egoist/mygo/ui"
)

type fakeLicenseService struct {
	activateErr   error
	validateErr   error
	deactivateErr error
	activated     []string
	validations   int
	deactivations int
}

var licenseTestNow = time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)

func (f *fakeLicenseService) Activate(_ context.Context, key string) (*license.State, error) {
	f.activated = append(f.activated, key)
	if f.activateErr != nil {
		return nil, f.activateErr
	}
	exp := time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC)
	return &license.State{Version: 1, Key: key, DisplayKey: "****-ABCD", ActivationID: "act-1",
		ActivatedAt: licenseTestNow, ExpiresAt: &exp, LastValidatedAt: licenseTestNow}, nil
}

func (f *fakeLicenseService) Validate(_ context.Context, s *license.State) error {
	f.validations++
	if f.validateErr != nil {
		if f.validateErr == license.ErrNotFound {
			s.Revoked = true
		}
		return f.validateErr
	}
	s.LastValidatedAt = licenseTestNow
	return nil
}

func (f *fakeLicenseService) Deactivate(context.Context, *license.State) error {
	f.deactivations++
	return f.deactivateErr
}

func newLicenseTestApp(t *testing.T, state *license.State) (*app, *fakeLicenseService) {
	t.Helper()
	a := newMessengerTestApp(t)
	fake := &fakeLicenseService{}
	a.license = licenseUI{enabled: true, client: fake, state: state, now: func() time.Time { return licenseTestNow },
		cfg: license.Config{OrganizationID: "11111111-2222-3333-4444-555555555555", BuildDate: time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)}}
	if state != nil {
		if err := license.Save(a.dataDir, state); err != nil {
			t.Fatal(err)
		}
	}
	a.settingsOpen = true
	a.settingsTab = 0
	return a, fake
}

func savedLicense(t *testing.T, a *app) *license.State {
	t.Helper()
	s, err := license.Load(a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func activeLicense(validated time.Time, expires time.Time) *license.State {
	return &license.State{Version: 1, Key: "AETHER-KEY", DisplayKey: "****-ABCD", ActivationID: "act-1",
		ActivatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), ExpiresAt: &expires, LastValidatedAt: validated}
}

func TestLicenseCardHiddenWhenLicensingIsDisabled(t *testing.T) {
	a := newMessengerTestApp(t)
	a.settingsOpen = true
	tester := ui.NewTester(a.view, 1120, 760)
	if tester.HasText("AETHER Pro") || !tester.HasText("Appearance") {
		t.Fatalf("disabled build shows the license card: %q", tester.Texts())
	}
}

func TestLicenseCardActivatesAndSaves(t *testing.T) {
	a, fake := newLicenseTestApp(t, nil)
	tester := ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("AETHER Pro") || !tester.HasText("Free") {
		t.Fatalf("license card missing: %q", tester.Texts())
	}
	if err := tester.Click("Activate"); err != nil {
		t.Fatal(err)
	}
	if len(fake.activated) != 0 {
		t.Fatal("activated with an empty key")
	}
	a.license.keyDraft = "  AETHER-KEY  "
	tester = ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Activate"); err != nil {
		t.Fatal(err)
	}
	if len(fake.activated) != 1 || fake.activated[0] != "AETHER-KEY" || a.license.keyDraft != "" {
		t.Fatalf("activated %q, draft %q", fake.activated, a.license.keyDraft)
	}
	if s := savedLicense(t, a); s == nil || s.ActivationID != "act-1" {
		t.Fatalf("saved license = %+v", s)
	}
	tester = ui.NewTester(a.view, 1120, 760)
	all := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(all, "Pro is active") || !strings.Contains(all, "****-ABCD") || !strings.Contains(all, "Paid through March 1, 2028") {
		t.Fatalf("active license not shown: %q", all)
	}
	if strings.Contains(all, "AETHER-KEY") {
		t.Fatalf("full key shown: %q", all)
	}
}

func TestLicenseCardActivationLimitExplainsAndSavesNothing(t *testing.T) {
	a, fake := newLicenseTestApp(t, nil)
	fake.activateErr = license.ErrNotPermitted
	a.license.keyDraft = "AETHER-KEY"
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Activate"); err != nil {
		t.Fatal(err)
	}
	if savedLicense(t, a) != nil || a.license.state != nil {
		t.Fatal("a refused key was saved")
	}
	tester = ui.NewTester(a.view, 1120, 760)
	if !strings.Contains(strings.Join(tester.Texts(), "\n"), "maximum number of devices") {
		t.Fatalf("refusal not explained: %q", tester.Texts())
	}
}

func TestLicenseCardCheckOfflineKeepsStateAndRevocationIsSaved(t *testing.T) {
	validated := licenseTestNow.Add(-3 * 24 * time.Hour)
	a, fake := newLicenseTestApp(t, activeLicense(validated, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)))

	fake.validateErr = license.ErrUnavailable
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Check now"); err != nil {
		t.Fatal(err)
	}
	if s := savedLicense(t, a); !s.LastValidatedAt.Equal(validated) || s.Revoked {
		t.Fatalf("offline check changed the saved license: %+v", s)
	}
	tester = ui.NewTester(a.view, 1120, 760)
	if !strings.Contains(strings.Join(tester.Texts(), "\n"), "Couldn't reach the license service") || !tester.HasText("Pro") {
		t.Fatalf("offline check: %q", tester.Texts())
	}

	fake.validateErr = license.ErrNotFound
	if err := tester.Click("Check now"); err != nil {
		t.Fatal(err)
	}
	if s := savedLicense(t, a); !s.Revoked {
		t.Fatal("revocation was not saved")
	}
	tester = ui.NewTester(a.view, 1120, 760)
	all := strings.Join(tester.Texts(), "\n")
	if !strings.Contains(all, "no longer recognises this license") || !tester.HasText("Free") {
		t.Fatalf("revoked license: %q", all)
	}
}

func TestLicenseCardShowsFallbackForAnExpiredYear(t *testing.T) {
	// Paid through Feb 15, 2027; this build is from Feb 1, 2027; now is Mar 1.
	a, _ := newLicenseTestApp(t, activeLicense(time.Date(2027, 2, 10, 0, 0, 0, 0, time.UTC), time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC)))
	tester := ui.NewTester(a.view, 1120, 760)
	all := strings.Join(tester.Texts(), "\n")
	if !tester.HasText("Pro · this version") || !strings.Contains(all, "This version keeps Pro for good") {
		t.Fatalf("fallback not shown: %q", all)
	}
	a.license.cfg.BuildDate = time.Date(2027, 2, 20, 0, 0, 0, 0, time.UTC)
	tester = ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Free") || !strings.Contains(strings.Join(tester.Texts(), "\n"), "came out after your license year ended") {
		t.Fatalf("newer build not explained: %q", tester.Texts())
	}
}

func TestLicenseCardRemove(t *testing.T) {
	a, fake := newLicenseTestApp(t, activeLicense(licenseTestNow, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)))
	fake.deactivateErr = license.ErrUnavailable
	tester := ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Remove from this Mac"); err != nil {
		t.Fatal(err)
	}
	if savedLicense(t, a) == nil || a.license.state == nil {
		t.Fatal("license removed although its activation was not released")
	}
	fake.deactivateErr = nil
	tester = ui.NewTester(a.view, 1120, 760)
	if err := tester.Click("Remove from this Mac"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "license.json")); !os.IsNotExist(err) || a.license.state != nil {
		t.Fatalf("license file still present: %v", err)
	}
	tester = ui.NewTester(a.view, 1120, 760)
	if !tester.HasText("Activate") {
		t.Fatalf("key field not back after removal: %q", tester.Texts())
	}
}

func TestLicenseStartupRecheckOnlyWhenStale(t *testing.T) {
	a, fake := newLicenseTestApp(t, activeLicense(licenseTestNow.Add(-2*time.Hour), time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)))
	a.recheckLicenseIfStale()
	if fake.validations != 0 {
		t.Fatal("rechecked a license validated two hours ago")
	}
	a.license.state.LastValidatedAt = licenseTestNow.Add(-30 * time.Hour)
	fake.validateErr = license.ErrUnavailable
	a.recheckLicenseIfStale()
	if fake.validations != 1 || a.license.err != "" {
		t.Fatalf("validations=%d err=%q: an offline startup check must stay silent", fake.validations, a.license.err)
	}
}
