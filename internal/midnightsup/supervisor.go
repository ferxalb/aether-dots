// Package midnightsup is an opt-in, explicit supervisor for one local
// Midnight server. It only ever launches a Midnight binary the user approved
// by SHA-256, never a command taken from MCP server configuration, and it
// passes no credentials to the process (tokens live in Midnight's own
// registry and in Dots' private token files).
package midnightsup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	// ErrNotApproved means no Midnight binary has been approved yet.
	ErrNotApproved = errors.New("no Midnight binary approved; run `aether midnight server approve`")
	// ErrCorrupt means supervisor state on disk is unreadable or invalid. It is
	// reported, never silently repaired, because the state may be the only
	// record of a running server.
	ErrCorrupt = errors.New("corrupt supervisor state")
)

// Config is what the user approved. Port is fixed after the first start so
// restarts keep the same endpoint and existing Dot connections stay valid.
type Config struct {
	Binary string `json:"binary"`
	SHA256 string `json:"sha256"`
	DB     string `json:"db"`
	Port   int    `json:"port,omitempty"`
}

type state struct {
	PID       int       `json:"pid"`
	Endpoint  string    `json:"endpoint"`
	StartedAt time.Time `json:"started_at"`
}

// Status describes the supervised server.
type Status struct {
	Approved  bool
	Config    Config
	Running   bool
	PID       int
	Endpoint  string
	StartedAt time.Time
	// AuthEnforced is true when an unauthenticated request was answered 401.
	AuthEnforced bool
}

func root(dataDir string) string { return filepath.Join(dataDir, "midnight") }

func ensureRoot(dataDir string) (string, error) {
	dir := root(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("supervisor directory must be a private plain directory")
	}
	return dir, nil
}

func lock(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "supervisor.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func writeJSON(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// openVerified opens the binary, checks it is a safe regular file and returns
// the descriptor with its hash. Callers exec that same descriptor, so the
// bytes that were hashed are the bytes that run.
func openVerified(path string) (*os.File, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", errors.New("Midnight binary path must be absolute")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, "", err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		f.Close()
		return nil, "", errors.New("Midnight binary must be an executable regular file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		f.Close()
		return nil, "", errors.New("Midnight binary must not be writable by group or others")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return nil, "", err
	}
	return f, hex.EncodeToString(h.Sum(nil)), nil
}

// Approve records the binary and database the user chose after verifying that
// the binary identifies itself as Midnight. It stops nothing and starts nothing.
func Approve(dataDir, binary, db string) (Config, error) {
	dir, err := ensureRoot(dataDir)
	if err != nil {
		return Config{}, err
	}
	unlock, err := lock(dir)
	if err != nil {
		return Config{}, err
	}
	defer unlock.Close()
	if !filepath.IsAbs(db) {
		return Config{}, errors.New("Midnight database path must be absolute")
	}
	f, sum, err := openVerified(binary)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	out, err := runVersion(f, binary)
	if err != nil {
		return Config{}, err
	}
	if !strings.HasPrefix(out, "aether-midnight ") {
		return Config{}, fmt.Errorf("binary does not identify as aether-midnight (%q)", firstLine(out))
	}
	real, _ := filepath.EvalSymlinks(binary)
	cfg := Config{Binary: real, SHA256: sum, DB: db}
	var previous Config
	if readJSON(filepath.Join(dir, "approved.json"), &previous) == nil {
		cfg.Port = previous.Port // keep endpoints stable across re-approval
	}
	return cfg, writeJSON(filepath.Join(dir, "approved.json"), cfg)
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func runVersion(f *os.File, path string) (string, error) {
	cmd := command(f, path, "--version")
	cmd.Env = []string{}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return "", err
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return "", errors.New("Midnight --version timed out")
	}
	return out.String(), nil
}

var hexSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

func loadConfig(dir string) (Config, error) {
	path := filepath.Join(dir, "approved.json")
	var cfg Config
	if err := readJSON(path, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, ErrNotApproved
		}
		return cfg, fmt.Errorf("%w: %s: %v (remove it and re-run `aether midnight server approve`)", ErrCorrupt, path, err)
	}
	if !filepath.IsAbs(cfg.Binary) || !filepath.IsAbs(cfg.DB) || !hexSHA.MatchString(cfg.SHA256) || cfg.Port < 0 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("%w: %s has invalid fields (remove it and re-approve)", ErrCorrupt, path)
	}
	return cfg, nil
}

// loadState returns (state, true, nil) for a readable state file,
// (zero, false, nil) when there is none, and ErrCorrupt when it is unreadable.
func loadState(dir string) (state, bool, error) {
	path := filepath.Join(dir, "server.json")
	var s state
	if err := readJSON(path, &s); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, false, nil
		}
		return s, false, fmt.Errorf("%w: %s: %v (a Midnight process may still be running; stop it manually, then remove the file)", ErrCorrupt, path, err)
	}
	if s.PID <= 1 || !strings.HasPrefix(s.Endpoint, "http://") {
		return s, false, fmt.Errorf("%w: %s has invalid fields (a Midnight process may still be running; stop it manually, then remove the file)", ErrCorrupt, path)
	}
	return s, true, nil
}

// owned reports whether pid is a live process running the approved Midnight
// binary in serve-http mode on the approved database. A recycled pid, or any
// other program, is never signalled.
func owned(pid int, cfg Config) bool {
	if pid <= 1 {
		return false
	}
	exe, args, ok := processIdentity(pid)
	if !ok || exe != cfg.Binary {
		return false
	}
	hasServe, hasDB := false, false
	for i, a := range args {
		hasServe = hasServe || a == "serve-http"
		hasDB = hasDB || (a == "--db" && i+1 < len(args) && args[i+1] == cfg.DB)
	}
	return hasServe && hasDB
}

func probe(endpoint string) bool {
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(endpoint, "application/json", strings.NewReader("{}"))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusUnauthorized
}

// Get reports the current state without changing anything. An unapproved
// supervisor is a valid state; corrupt files are an error.
func Get(dataDir string) (Status, error) {
	dir := root(dataDir)
	cfg, err := loadConfig(dir)
	if errors.Is(err, ErrNotApproved) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	st := Status{Approved: true, Config: cfg}
	s, ok, err := loadState(dir)
	if err != nil {
		return st, err
	}
	if ok && owned(s.PID, cfg) {
		st.Running, st.PID, st.Endpoint, st.StartedAt = true, s.PID, s.Endpoint, s.StartedAt
		st.AuthEnforced = probe(s.Endpoint)
	}
	return st, nil
}

// Start launches the approved Midnight binary unless one is already running.
func Start(dataDir string) (Status, error) {
	dir, err := ensureRoot(dataDir)
	if err != nil {
		return Status{}, err
	}
	unlock, err := lock(dir)
	if err != nil {
		return Status{}, err
	}
	defer unlock.Close()
	cfg, err := loadConfig(dir)
	if err != nil {
		return Status{}, err
	}
	statePath := filepath.Join(dir, "server.json")
	s, ok, err := loadState(dir)
	if err != nil {
		return Status{}, err
	}
	if ok {
		if owned(s.PID, cfg) {
			return Status{Approved: true, Config: cfg, Running: true, PID: s.PID, Endpoint: s.Endpoint, StartedAt: s.StartedAt, AuthEnforced: probe(s.Endpoint)}, nil
		}
		_ = os.Remove(statePath) // stale: process gone or pid recycled
	}
	f, sum, err := openVerified(cfg.Binary)
	if err != nil {
		return Status{}, err
	}
	defer f.Close()
	if sum != cfg.SHA256 {
		return Status{}, errors.New("Midnight binary changed since it was approved; re-run `aether midnight server approve`")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DB), 0o700); err != nil {
		return Status{}, err
	}
	logPath := filepath.Join(dir, "server.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Status{}, err
	}
	defer logf.Close()
	cmd := command(f, cfg.Binary, "--db", cfg.DB, "serve-http", "--bind", "127.0.0.1:"+strconv.Itoa(cfg.Port))
	// No inherited environment: nothing credential-shaped can reach the server.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + os.Getenv("HOME")}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Status{}, err
	}
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	// Any failure after this point must leave nothing behind: the child is
	// killed and reaped, and no state file claims it.
	abort := func(cause error) (Status, error) {
		_ = cmd.Process.Signal(syscall.SIGINT)
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		_ = os.Remove(statePath)
		return Status{}, cause
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := os.ReadFile(logPath)
		if line := listeningLine(string(raw)); line != "" {
			u, err := url.Parse(line)
			if err != nil || !isLoopback(u.Host) {
				return abort(errors.New("Midnight reported a non-loopback address; stopped"))
			}
			_, portStr, _ := net.SplitHostPort(u.Host)
			port, _ := strconv.Atoi(portStr)
			if cfg.Port == 0 {
				// Persist the port first so a restart keeps the endpoint.
				fixed := cfg
				fixed.Port = port
				if err := writeJSON(filepath.Join(dir, "approved.json"), fixed); err != nil {
					return abort(err)
				}
				cfg = fixed
			}
			started := time.Now().UTC()
			if err := writeJSON(statePath, state{PID: pid, Endpoint: line, StartedAt: started}); err != nil {
				return abort(err)
			}
			_ = cmd.Process.Release()
			return Status{Approved: true, Config: cfg, Running: true, PID: pid, Endpoint: line, StartedAt: started, AuthEnforced: probe(line)}, nil
		}
		select {
		case <-exited:
			raw, _ := os.ReadFile(logPath)
			_ = os.Remove(statePath)
			return Status{}, fmt.Errorf("Midnight exited during startup: %s", firstLine(string(raw)))
		case <-time.After(10 * time.Millisecond):
		}
	}
	return abort(errors.New("Midnight did not report a listening address in time"))
}

func listeningLine(log string) string {
	for _, l := range strings.Split(log, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), "listening "); ok && strings.HasSuffix(rest, "/mcp") {
			return rest
		}
	}
	return ""
}

func isLoopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Stop asks the supervised server to exit (SIGINT, which Midnight handles as a
// clean shutdown) and escalates only against the verified process.
func Stop(dataDir string, timeout time.Duration) (bool, error) {
	dir, err := ensureRoot(dataDir)
	if err != nil {
		return false, err
	}
	unlock, err := lock(dir)
	if err != nil {
		return false, err
	}
	defer unlock.Close()
	cfg, err := loadConfig(dir)
	if err != nil {
		return false, err
	}
	statePath := filepath.Join(dir, "server.json")
	s, ok, err := loadState(dir)
	if err != nil {
		return false, err
	}
	if !ok || !owned(s.PID, cfg) {
		_ = os.Remove(statePath)
		return false, nil
	}
	_ = syscall.Kill(s.PID, syscall.SIGINT)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !owned(s.PID, cfg) {
			_ = os.Remove(statePath)
			return true, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	if owned(s.PID, cfg) {
		_ = syscall.Kill(s.PID, syscall.SIGKILL)
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(statePath)
	return true, nil
}
