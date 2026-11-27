package license

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func withLicensing(t *testing.T) {
	t.Helper()
	org, env, build := OrganizationID, Environment, BuildDate
	t.Cleanup(func() { OrganizationID, Environment, BuildDate = org, env, build })
	OrganizationID, Environment, BuildDate = testOrg, "", "2027-01-01T00:00:00Z"
}

func TestCurrentFeatures(t *testing.T) {
	dir := t.TempDir()
	now := date("2027-03-01")

	withLicensing(t)
	OrganizationID = ""
	if f := CurrentFeatures(dir, now); f.Licensing || !f.Pro {
		t.Fatalf("build without licensing = %+v, want every feature", f)
	}
	OrganizationID = testOrg
	if f := CurrentFeatures(dir, now); !f.Licensing || f.Pro {
		t.Fatalf("no license = %+v, want Free", f)
	}
	if err := Save(dir, &State{Version: stateVersion, Key: testKey, ActivationID: testActivation, ExpiresAt: ptr(date("2028-01-01")), LastValidatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if f := CurrentFeatures(dir, now); !f.Pro {
		t.Fatalf("active license = %+v, want Pro", f)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f := CurrentFeatures(dir, now); f.Pro {
		t.Fatal("a corrupt license file unlocked Pro")
	}
}

func TestFeatureGates(t *testing.T) {
	free, pro := Features{Licensing: true}, Features{Licensing: true, Pro: true}
	for existing := 0; existing < FreeAgentLimit; existing++ {
		if err := free.CanCreateAgent(existing); err != nil {
			t.Fatalf("Free refused agent %d: %v", existing+1, err)
		}
	}
	if err := free.CanCreateAgent(FreeAgentLimit); !errors.Is(err, ErrProRequired) {
		t.Fatalf("Free created agent %d: %v", FreeAgentLimit+1, err)
	}
	if err := pro.CanCreateAgent(500); err != nil {
		t.Fatalf("Pro limited agents: %v", err)
	}
	if err := free.Require("Routines"); !errors.Is(err, ErrProRequired) || err.Error() != "Routines requires AETHER Pro" {
		t.Fatalf("Require = %v", err)
	}
	if err := pro.Require("Routines"); err != nil {
		t.Fatal(err)
	}
}
