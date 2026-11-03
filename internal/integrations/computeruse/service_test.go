package computeruse

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestInstallerCommandUsesFixedChecksummedStableReleaseWithoutCosignPrerequisite(t *testing.T) {
	for _, expected := range []string{
		"set -o pipefail",
		"curl -fsSL https://cua.ai/driver/install.sh",
		"--channel stable",
		"--no-modify-path",
	} {
		if !strings.Contains(cuadriverInstall, expected) {
			t.Errorf("installer command is missing %q: %q", expected, cuadriverInstall)
		}
	}
	if strings.Contains(cuadriverInstall, "--require-signature") {
		t.Fatalf("installer command reintroduced the undeclared cosign requirement: %q", cuadriverInstall)
	}
}

func TestLimitedBufferBoundsAndSanitizesInstallerDiagnostic(t *testing.T) {
	var output limitedBuffer
	input := "curl: failed\x00\x01\n" + strings.Repeat("x", maxInstallerOutput+10)
	written, err := output.Write([]byte(input))
	if err != nil || written != len(input) {
		t.Fatalf("Write() = (%d, %v), want (%d, nil)", written, err, len(input))
	}
	got := output.String()
	if strings.ContainsAny(got, "\x00\x01") {
		t.Fatalf("diagnostic retained control bytes: %q", got)
	}
	if !strings.HasPrefix(got, "curl: failed\n") || !strings.HasSuffix(got, " …") {
		t.Fatalf("diagnostic was not preserved and marked as truncated: %q", got)
	}
	if len(got) > maxInstallerOutput+4 {
		t.Fatalf("diagnostic exceeded its output bound: %d bytes", len(got))
	}
}

func TestCommandFailureIncludesInstallerDiagnostic(t *testing.T) {
	err := commandFailure("Cua Driver installer", errors.New("exit status 23"), "curl: HTTP 403")
	if err == nil || !strings.Contains(err.Error(), "curl: HTTP 403") || !strings.Contains(err.Error(), "23") {
		t.Fatalf("installer failure lost its cause: %v", err)
	}
}

func TestDriverEnvironmentDropsInstallerOverrides(t *testing.T) {
	env := driverEnvironment([]string{
		"PATH=/usr/bin",
		"LANG=en_US.UTF-8",
		"CUA_DRIVER_RS_HOME=/tmp/attacker-controlled",
		"CUA_DRIVER_POLICY_FILE=/tmp/attacker-policy.yaml",
	}, "/private/aether/policy.yaml")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "LANG=en_US.UTF-8") {
		t.Fatalf("driver environment dropped expected base settings: %q", env)
	}
	if strings.Contains(joined, "attacker-controlled") || strings.Contains(joined, "attacker-policy") {
		t.Fatalf("driver environment retained an inherited CUA override: %q", env)
	}
	if !strings.Contains(joined, "CUA_DRIVER_POLICY_FILE=/private/aether/policy.yaml") {
		t.Fatalf("driver environment omitted the explicit policy path: %q", env)
	}
}

func TestInstallerEnvironmentUsesFixedPathAndIgnoresCuaOverrides(t *testing.T) {
	t.Setenv("CUA_DRIVER_BIN_DIR", "/tmp/untrusted-bin")
	t.Setenv("CUA_DRIVER_RS_HOME", "/tmp/untrusted-home")
	env, err := installerEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "PATH=/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Fatalf("installer PATH is not fixed: %q", env)
	}
	if !strings.Contains(joined, "HOME="+mustHome(t)) {
		t.Fatalf("installer HOME does not use the current user's resolved home: %q", env)
	}
	if strings.Contains(joined, "CUA_DRIVER_") || strings.Contains(joined, "untrusted-") {
		t.Fatalf("installer environment retained a CUA override: %q", env)
	}
}

func mustHome(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return home
}
