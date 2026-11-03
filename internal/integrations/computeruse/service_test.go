package computeruse

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestBoundedDaemonLaunchUsesLaunchServicesAppIdentity(t *testing.T) {
	args := strings.Join(boundedDaemonLaunchArgs("/private/aether/cua.sock", "/private/aether/manifest.yaml"), " ")
	for _, expected := range []string{
		"-a " + cuadriverAppPath,
		"--args serve",
		"--socket /private/aether/cua.sock",
		"--permission-mode bounded",
		"--capability-manifest /private/aether/manifest.yaml",
		"--approve-capability-manifest",
		"--no-permissions-gate",
	} {
		if !strings.Contains(args, expected) {
			t.Errorf("bounded daemon launch is missing %q: %q", expected, args)
		}
	}
}

func TestComputerUseAvoidsGlobalAppDiscovery(t *testing.T) {
	if strings.Contains(capabilityManifest, "list_apps") || strings.Contains(driverPolicy, "list_apps") {
		t.Fatal("the Computer Use capability manifest must not allow global app discovery")
	}
	if allowedDriverTool("list_apps") {
		t.Fatal("global app discovery must not be callable through AETHER")
	}
	if !allowedDriverTool("launch_app") || !allowedDriverTool("list_windows") {
		t.Fatal("scoped app launch and window listing must remain available")
	}
}

func TestMusicClickUsesOnlyFieldsSupportedByCuaDriver(t *testing.T) {
	args := musicClickArguments("aether-test", "s0000002a:4")
	if len(args) != 3 || args["session"] != "aether-test" || args["element_token"] != "s0000002a:4" || args["action"] != "press" {
		t.Fatalf("music click arguments = %#v, want session, element_token, and press action", args)
	}
	if strings.Contains(driverPolicy, "post_action_wait") {
		t.Fatal("driver policy contains the removed post_action_wait click field")
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

func TestAllowedDriverBinaryPathAcceptsOnlyManagedOrSignedAppExecutable(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	packageBinary := filepath.Join(home, ".cua-driver", "packages", "releases", "0.38.0", "cua-driver")
	appBinary := filepath.Join(root, "Applications", "CuaDriver.app", "Contents", "MacOS", "cua-driver")
	outsideBinary := filepath.Join(root, "other", "cua-driver")
	for _, path := range []string{packageBinary, appBinary, outsideBinary} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test executable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	packageResolved, err := filepath.EvalSymlinks(packageBinary)
	if err != nil {
		t.Fatal(err)
	}
	appResolved, err := filepath.EvalSymlinks(appBinary)
	if err != nil {
		t.Fatal(err)
	}
	outsideResolved, err := filepath.EvalSymlinks(outsideBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !allowedDriverBinaryPath(home, packageResolved, appBinary) {
		t.Fatal("managed package executable was rejected")
	}
	if !allowedDriverBinaryPath(home, appResolved, appBinary) {
		t.Fatal("executable inside the signed CuaDriver app was rejected")
	}
	if allowedDriverBinaryPath(home, outsideResolved, appBinary) {
		t.Fatal("unmanaged executable outside both approved locations was accepted")
	}
}

func TestInstalledCuaDriverUsesApprovedAppLayoutAndValidSignature(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Cua Driver app installation is macOS-only")
	}
	service := New(t.TempDir())
	if !service.Installed() {
		t.Skip("Cua Driver is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := verifyAppSignature(ctx); err != nil {
		t.Fatalf("installed Cua Driver signature verification failed: %v", err)
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
