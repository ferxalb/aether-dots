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
	cuadriverAppPath = "/Applications/CuaDriver.app"
	cuadriverInstall = "curl -fsSL https://cua.ai/driver/install.sh | /bin/bash -s -- --channel stable --require-signature --no-modify-path"
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
)

const (
	notesBundleID = "com.apple.Notes"
	musicBundleID = "com.apple.Music"
)

type Service struct {
	dataDir string

	mu     sync.Mutex
	client *mcpClient
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
// filters installer overrides so the fixed stable, signature-checked install
// cannot be redirected by inherited CUA_DRIVER_* variables.
func (s *Service) Install(ctx context.Context) error {
	if runtime.GOOS != "darwin" {
		return errors.New("Computer Use is available only in the macOS desktop app")
	}
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", cuadriverInstall)
	cmd.Env = installerEnvironment(os.Environ())
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("Cua Driver could not be installed; check your connection and try again")
	}
	if !s.Installed() {
		return errors.New("the Cua Driver installer finished without an app at its expected install path")
	}
	if err := verifyAppSignature(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) OpenAccessibilitySettings() error {
	if runtime.GOOS != "darwin" {
		return errors.New("Accessibility settings are available only on macOS")
	}
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
	s.mu.Lock()
	client := s.client
	s.client = nil
	s.mu.Unlock()
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

func (s *Service) ObserveNotes(ctx context.Context, session string) (string, error) {
	window, state, err := s.observeApp(ctx, session, notesBundleID)
	if err != nil {
		return "", err
	}
	return encodeUntrustedObservation("Apple Notes", window, state)
}

func (s *Service) ControlMusic(ctx context.Context, session, action string) (string, error) {
	allowed := map[string]bool{"play": true, "pause": true, "next": true, "previous": true}
	if !allowed[action] {
		return "", errors.New("Music action is not allow-listed")
	}
	window, state, err := s.observeApp(ctx, session, musicBundleID)
	if err != nil {
		return "", err
	}
	targetLabel, oppositeLabel := musicLabels(action)
	if action == "play" || action == "pause" {
		tokens := matchingButtonTokens(state, oppositeLabel)
		if len(tokens) == 1 {
			return fmt.Sprintf("Music is already %s.", oppositeLabel), nil
		}
	}
	tokens := matchingButtonTokens(state, targetLabel)
	if len(tokens) != 1 {
		return "", errors.New("Music did not expose one unambiguous playback control for that action")
	}
	if _, err := s.callTool(ctx, "click", map[string]any{
		"element_token":   tokens[0],
		"session":         session,
		"action":          "press",
		"post_action_wait": "none",
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Sent the %q playback action to Music window %q.", action, window), nil
}

func (s *Service) observeApp(ctx context.Context, session, bundleID string) (string, map[string]any, error) {
	if bundleID != notesBundleID && bundleID != musicBundleID {
		return "", nil, errors.New("Computer Use app is not allow-listed")
	}
	if session == "" || len(session) > 64 {
		return "", nil, errors.New("Computer Use session is invalid")
	}
	if err := s.startSession(ctx, session); err != nil {
		return "", nil, err
	}
	appsPayload, err := s.callTool(ctx, "list_apps", map[string]any{})
	if err != nil {
		return "", nil, err
	}
	pid, running := findAppPID(appsPayload, bundleID)
	if !running {
		if _, err := s.callTool(ctx, "launch_app", map[string]any{"bundle_id": bundleID, "session": session}); err != nil {
			return "", nil, err
		}
		pid, running = s.waitForApp(ctx, bundleID)
		if !running {
			return "", nil, errors.New("the approved macOS app did not become available")
		}
	}
	windowsPayload, err := s.callTool(ctx, "list_windows", map[string]any{"pid": pid, "on_screen_only": true, "session": session})
	if err != nil {
		return "", nil, err
	}
	windowID, title, found := findUniqueWindow(windowsPayload)
	if !found {
		return "", nil, errors.New("open one visible window in the approved app and try again")
	}
	statePayload, err := s.callTool(ctx, "get_window_state", map[string]any{
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

func (s *Service) waitForApp(ctx context.Context, bundleID string) (int64, bool) {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		payload, err := s.callTool(ctx, "list_apps", map[string]any{})
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

func (s *Service) startSession(ctx context.Context, session string) error {
	_, err := s.callTool(ctx, "start_session", map[string]any{"session": session})
	return err
}

func (s *Service) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if !allowedDriverTool(name) {
		return nil, errors.New("Cua Driver tool is not allow-listed")
	}
	client, err := s.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	result, err := client.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, errors.New("Cua Driver refused the Computer Use action; check Accessibility access and the app's current state")
	}
	if result.Structured != nil {
		var payload any
		if err := json.Unmarshal(result.Structured, &payload); err != nil {
			return nil, errors.New("Cua Driver returned invalid structured data")
		}
		return payload, nil
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

func (s *Service) ensureClient(ctx context.Context) (*mcpClient, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("Computer Use is available only in the macOS desktop app")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
		return nil
	}
	manifestPath := s.manifestPath()
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		return errors.New("could not prepare the Computer Use capability manifest")
	}
	if err := os.WriteFile(manifestPath, []byte(capabilityManifest), 0o600); err != nil {
		return errors.New("could not write the Computer Use capability manifest")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/open", "-n", "-g", cuadriverAppPath, "--args",
		"serve", "--socket", s.socketPath(), "--permission-mode", "bounded",
		"--capability-manifest", manifestPath, "--approve-capability-manifest")
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
	return path, nil
}

func (s *Service) socketPath() string {
	return filepath.Join(s.dataDir, "computer-use", "aether-cua.sock")
}

func (s *Service) manifestPath() string {
	return filepath.Join(s.dataDir, "computer-use", "capability.yaml")
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

func installerEnvironment(source []string) []string {
	env := make([]string, 0, len(source))
	for _, item := range source {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.HasPrefix(key, "CUA_DRIVER_") {
			continue
		}
		env = append(env, item)
	}
	return env
}

func allowedDriverTool(name string) bool {
	switch name {
	case "start_session", "end_session", "launch_app", "list_apps", "list_windows", "get_window_state", "click":
		return true
	default:
		return false
	}
}

