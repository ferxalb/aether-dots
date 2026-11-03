package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	cuadriverAppPath   = "/Applications/CuaDriver.app"
	cuadriverInstall   = "curl -fsSL https://cua.ai/driver/install.sh | /bin/bash -s -- --channel stable --require-signature --no-modify-path"
	capabilityManifest = `version: 3
expires_after: 8h
idle_timeout: 30m

allow:
  tools:
    - start_session
    - end_session
    - launch_app
    - list_apps
    - list_windows
    - get_window_state
    - click

resources:
  apps:
    - bundle_id: com.apple.Notes
      launch: true
      windows: all
    - bundle_id: com.apple.Music
      launch: true
      windows: all
  desktop:
    display: false
`
	driverPolicy = `allow:
  tools:
    - start_session
    - end_session
    - list_apps
    - list_windows
  rules:
    - tool: launch_app
      constraints:
        bundle_id:
          allowed: ["com.apple.Notes", "com.apple.Music"]
    - tool: get_window_state
      constraints:
        include_screenshot:
          allowed: [false]
        include_accessibility_tree:
          allowed: [true]
        max_elements:
          max: 120
        max_depth:
          max: 10
    - tool: click
      constraints:
        action:
          allowed: ["press"]
        post_action_wait:
          allowed: ["none"]
`
)

const (
	notesBundleID = "com.apple.Notes"
	musicBundleID = "com.apple.Music"
)

type Service struct {
	dataDir string

	mu         sync.Mutex
	client     *mcpClient
	generation uint64
}

func New(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

func (s *Service) Installed() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	driver, err := s.driverPath()
	if err != nil {
		return false
	}
	if info, err := os.Stat(driver); err != nil || info.IsDir() {
		return false
	}
	if info, err := os.Stat(cuadriverAppPath); err != nil || !info.IsDir() {
		return false
	}
	return true
}

// Install runs only after a user clicks the integration setup button. It
// passes a fixed stable, signature-checked install only the environment it
// needs; shell startup and installer override variables are not inherited.
func (s *Service) Install(ctx context.Context) error {
	if runtime.GOOS != "darwin" {
		return errors.New("Computer Use is available only in the macOS desktop app")
	}
	cmd := exec.CommandContext(ctx, "/bin/bash", "--noprofile", "--norc", "-c", cuadriverInstall)
	cmd.Env = installerEnvironment()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Cua Driver could not be installed; check your connection and try again")
	}
	if !s.Installed() {
		return errors.New("the Cua Driver installer finished without an app at its expected install path")
	}
	if err := verifyAppSignature(ctx); err != nil {
		return err
	}
	driver, err := s.driverPath()
	if err != nil {
		return err
	}
	telemetry := exec.CommandContext(ctx, driver, "telemetry", "disable")
	telemetry.Env = driverEnvironment(os.Environ(), "")
	telemetry.Stdout = io.Discard
	telemetry.Stderr = io.Discard
	if err := telemetry.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Cua Driver installed, but its optional usage telemetry could not be disabled")
	}
	s.Stop()
	return nil
}

func (s *Service) OpenAccessibilitySettings() error {
	if runtime.GOOS != "darwin" {
		return errors.New("Accessibility settings are available only on macOS")
	}
	// Cua Driver must be relaunched after a TCC grant changes. Stop AETHER's
	// bounded session before directing the user to the system settings pane.
	s.Stop()
	cmd := exec.Command("/usr/bin/open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return errors.New("could not open macOS Accessibility settings")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *Service) Stop() {
	s.stopRuntime()()
}

// StopAsync invalidates active tools immediately, then stops the driver off
// the UI thread.
func (s *Service) StopAsync() {
	stop := s.stopRuntime()
	go stop()
}

func (s *Service) stopRuntime() func() {
	s.mu.Lock()
	s.generation++
	client := s.client
	s.client = nil
	s.mu.Unlock()
	return func() {
		if client != nil {
			client.Close()
		}
		if runtime.GOOS != "darwin" {
			return
		}
		driver, err := s.driverPath()
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, driver, "stop", "--socket", s.socketPath())
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
}

// Generation returns the current authorization generation. Tool handlers bind
// to it so a revoked or stopped session cannot silently reconnect mid-run.
func (s *Service) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

// Prepare starts and warms the bounded MCP connection after user-requested
// installation, reducing first-use latency without reading any app data.
func (s *Service) Prepare(ctx context.Context) error {
	_, err := s.ensureClient(ctx, s.Generation())
	return err
}

func (s *Service) ObserveNotes(ctx context.Context, session string, generation uint64) (string, error) {
	window, state, err := s.observeApp(ctx, session, notesBundleID, generation)
	if err != nil {
		return "", err
	}
	if !s.isCurrent(generation) {
		return "", errors.New("Computer Use access was revoked during the Notes read")
	}
	return encodeUntrustedObservation("Apple Notes", window, state)
}

func (s *Service) ControlMusic(ctx context.Context, session, action string, generation uint64) (string, error) {
	allowed := map[string]bool{"play": true, "pause": true, "next": true, "previous": true}
	if !allowed[action] {
		return "", errors.New("Music action is not allow-listed")
	}
	window, state, err := s.observeApp(ctx, session, musicBundleID, generation)
	if err != nil {
		return "", err
	}
	if !s.isCurrent(generation) {
		return "", errors.New("Computer Use access was revoked before the Music action")
	}
	targetLabel, oppositeLabel := musicLabels(action)
	if action == "play" || action == "pause" {
		tokens := matchingButtonTokens(state, oppositeLabel)
		if len(tokens) == 1 {
			state := "playing"
			if action == "pause" {
				state = "paused"
			}
			return fmt.Sprintf("Music is already %s.", state), nil
		}
	}
	tokens := matchingButtonTokens(state, targetLabel)
	if len(tokens) != 1 {
		return "", errors.New("Music did not expose one unambiguous playback control for that action")
	}
	if !s.isCurrent(generation) {
		return "", errors.New("Computer Use access was revoked before the Music action")
	}
	if _, err := s.callTool(ctx, generation, "click", map[string]any{
		"element_token":    tokens[0],
		"session":          session,
		"action":           "press",
		"post_action_wait": "none",
	}); err != nil {
		return "", err
	}
	if !s.isCurrent(generation) {
		return "", errors.New("Computer Use access was revoked during the Music action")
	}
	return fmt.Sprintf("Sent the %q playback action to Music window %q.", action, window), nil
}

func (s *Service) observeApp(ctx context.Context, session, bundleID string, generation uint64) (string, map[string]any, error) {
	if bundleID != notesBundleID && bundleID != musicBundleID {
		return "", nil, errors.New("Computer Use app is not allow-listed")
	}
	if session == "" || len(session) > 64 {
		return "", nil, errors.New("Computer Use session is invalid")
	}
	if err := s.startSession(ctx, generation, session); err != nil {
		return "", nil, err
	}
	appsPayload, err := s.callTool(ctx, generation, "list_apps", map[string]any{})
	if err != nil {
		return "", nil, err
	}
	pid, running := findAppPID(appsPayload, bundleID)
	if !running {
		if _, err := s.callTool(ctx, generation, "launch_app", map[string]any{"bundle_id": bundleID, "session": session}); err != nil {
			return "", nil, err
		}
		pid, running = s.waitForApp(ctx, generation, bundleID)
		if !running {
			return "", nil, errors.New("the approved macOS app did not become available")
		}
	}
	windowsPayload, err := s.callTool(ctx, generation, "list_windows", map[string]any{"pid": pid, "on_screen_only": true, "session": session})
	if err != nil {
		return "", nil, err
	}
	windowID, title, found := findUniqueWindow(windowsPayload)
	if !found {
		return "", nil, errors.New("open one visible window in the approved app and try again")
	}
	statePayload, err := s.callTool(ctx, generation, "get_window_state", map[string]any{
		"pid": pid, "window_id": windowID, "session": session,
		"include_accessibility_tree": true, "include_screenshot": false,
		"tree_format": "elements", "max_elements": 120, "max_depth": 10,
	})
	if err != nil {
		return "", nil, err
	}
	state, ok := statePayload.(map[string]any)
	if !ok {
		return "", nil, errors.New("Cua Driver returned an unexpected window state")
	}
	return title, state, nil
}

func (s *Service) waitForApp(ctx context.Context, generation uint64, bundleID string) (int64, bool) {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		payload, err := s.callTool(ctx, generation, "list_apps", map[string]any{})
		if err == nil {
			if pid, running := findAppPID(payload, bundleID); running {
				return pid, true
			}
		}
		select {
		case <-ctx.Done():
			return 0, false
		case <-deadline.C:
			return 0, false
		case <-ticker.C:
		}
	}
}

func (s *Service) startSession(ctx context.Context, generation uint64, session string) error {
	_, err := s.callTool(ctx, generation, "start_session", map[string]any{"session": session})
	return err
}

func (s *Service) callTool(ctx context.Context, generation uint64, name string, args map[string]any) (any, error) {
	if !allowedDriverTool(name) {
		return nil, errors.New("Cua Driver tool is not allow-listed")
	}
	client, err := s.ensureClient(ctx, generation)
	if err != nil {
		return nil, err
	}
	result, err := client.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}
	if !s.isCurrent(generation) {
		return nil, errors.New("Computer Use access was revoked during the action")
	}
	if result.IsError {
		return nil, errors.New("Cua Driver refused the Computer Use action; check Accessibility access and the app's current state")
	}
	if result.Structured != nil {
		return result.Structured, nil
	}
	if result.Text != "" {
		var payload any
		if err := json.Unmarshal([]byte(result.Text), &payload); err == nil {
			return payload, nil
		}
		return result.Text, nil
	}
	return map[string]any{}, nil
}

func (s *Service) ensureClient(ctx context.Context, generation uint64) (*mcpClient, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("Computer Use is available only in the macOS desktop app")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != generation {
		return nil, errors.New("Computer Use authorization changed; start a new interactive run")
	}
	if s.client != nil && s.client.Alive() {
		return s.client, nil
	}
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	if !s.Installed() {
		return nil, errors.New("install Cua Driver in Settings → Integrations before granting Computer Use")
	}
	if err := verifyAppSignature(ctx); err != nil {
		return nil, err
	}
	if err := s.ensureDaemon(ctx); err != nil {
		return nil, err
	}
	driver, err := s.driverPath()
	if err != nil {
		return nil, err
	}
	client, err := newMCPClient(ctx, driver, s.socketPath())
	if err != nil {
		return nil, errors.New("could not connect to the bounded Cua Driver session")
	}
	s.client = client
	return client, nil
}

func (s *Service) ensureDaemon(ctx context.Context) error {
	driver, err := s.driverPath()
	if err != nil {
		return err
	}
	if s.daemonRunning(ctx, driver) {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(stopCtx, driver, "stop", "--socket", s.socketPath())
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		stopErr := cmd.Run()
		cancel()
		if stopErr != nil {
			return errors.New("could not replace the existing AETHER Cua Driver session")
		}
		deadline := time.NewTimer(2 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for s.daemonRunning(ctx, driver) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return errors.New("the previous AETHER Cua Driver session did not stop")
			case <-ticker.C:
			}
		}
	}
	manifestPath := s.manifestPath()
	if err := writePrivateConfig(manifestPath, []byte(capabilityManifest)); err != nil {
		return errors.New("could not write the Computer Use capability manifest")
	}
	policyPath := s.policyPath()
	if err := writePrivateConfig(policyPath, []byte(driverPolicy)); err != nil {
		return errors.New("could not write the Computer Use tool policy")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/open", "-n", "-g", cuadriverAppPath, "--args",
		"serve", "--socket", s.socketPath(), "--permission-mode", "bounded",
		"--capability-manifest", manifestPath, "--approve-capability-manifest", "--no-permissions-gate")
	cmd.Env = driverEnvironment(os.Environ(), policyPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("could not start Cua Driver with AETHER's bounded permission manifest")
	}
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.daemonRunning(ctx, driver) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Cua Driver did not start its bounded session")
		case <-ticker.C:
		}
	}
}

func (s *Service) daemonRunning(ctx context.Context, driver string) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, driver, "status", "--socket", s.socketPath())
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func (s *Service) driverPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("could not resolve the current user's home directory")
	}
	path := filepath.Join(home, ".local", "bin", "cua-driver")
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("Cua Driver is not installed")
	}
	packageRoot := filepath.Join(home, ".cua-driver", "packages")
	rel, err := filepath.Rel(packageRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("Cua Driver is outside its expected managed install directory")
	}
	return resolved, nil
}

func (s *Service) socketPath() string {
	return filepath.Join(s.dataDir, "computer-use", "aether-cua.sock")
}

func (s *Service) manifestPath() string {
	return filepath.Join(s.dataDir, "computer-use", "capability.yaml")
}

func (s *Service) policyPath() string {
	return filepath.Join(s.dataDir, "computer-use", "policy.yaml")
}

func verifyAppSignature(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", cuadriverAppPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("Cua Driver's macOS code signature could not be verified")
	}
	return nil
}

func installerEnvironment() []string {
	home, _ := os.UserHomeDir()
	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	if tempDir := os.TempDir(); tempDir != "" {
		env = append(env, "TMPDIR="+tempDir)
	}
	for _, key := range []string{"USER", "LOGNAME", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func driverEnvironment(source []string, policyPath string) []string {
	env := make([]string, 0, len(source)+1)
	for _, item := range source {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.HasPrefix(key, "CUA_DRIVER_") {
			continue
		}
		env = append(env, item)
	}
	if policyPath != "" {
		env = append(env, "CUA_DRIVER_POLICY_FILE="+policyPath)
	}
	return env
}

func writePrivateConfig(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Computer Use config directory is not a private directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".aether-cua-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (s *Service) isCurrent(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation == generation
}

func allowedDriverTool(name string) bool {
	switch name {
	case "start_session", "end_session", "launch_app", "list_apps", "list_windows", "get_window_state", "click":
		return true
	default:
		return false
	}
}
