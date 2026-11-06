package midnightsup

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func midnightBin(t *testing.T) string {
	bin := os.Getenv("MIDNIGHT_BIN")
	if bin == "" {
		t.Skip("MIDNIGHT_BIN not set")
	}
	return bin
}

func copyBin(t *testing.T, src string) string {
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	dst := filepath.Join(t.TempDir(), "aether-midnight")
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
	return dst
}

func TestApproveRejectsNonMidnightAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "m.db")
	if _, err := Approve(dir, "/bin/true", db); err == nil {
		t.Fatal("a binary that is not Midnight must not be approved")
	}
	if _, err := Approve(dir, "aether-midnight", db); err == nil {
		t.Fatal("relative binary path must be refused")
	}
	if _, err := Start(dir); err == nil || !strings.Contains(err.Error(), "approve") {
		t.Fatalf("start without approval must fail, got %v", err)
	}
}

func TestSupervisorLifecycle(t *testing.T) {
	bin := copyBin(t, midnightBin(t))
	dir := t.TempDir()
	db := filepath.Join(dir, "m.db")
	if _, err := Approve(dir, bin, db); err != nil {
		t.Fatal(err)
	}
	first, err := Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = Stop(dir, 3*time.Second) })
	if !first.Running || !first.AuthEnforced || first.PID <= 1 {
		t.Fatalf("unexpected start status %+v", first)
	}
	// A second start adopts the running server instead of launching another.
	second, err := Start(dir)
	if err != nil || second.PID != first.PID || second.Endpoint != first.Endpoint {
		t.Fatalf("duplicate start: %+v %v", second, err)
	}
	stopped, err := Stop(dir, 5*time.Second)
	if err != nil || !stopped {
		t.Fatalf("stop: %v %v", stopped, err)
	}
	if _, err := os.Stat("/proc/" + strconv.Itoa(first.PID)); err == nil {
		t.Fatal("process still alive after stop")
	}
	// Restart keeps the endpoint so existing Dot connections stay valid.
	again, err := Start(dir)
	if err != nil || again.Endpoint != first.Endpoint {
		t.Fatalf("restart endpoint changed: %+v %v", again, err)
	}
	// A stale pid in the state file (recycled pid) is never signalled.
	_, _ = Stop(dir, 3*time.Second)
	if err := writeJSON(filepath.Join(root(dir), "server.json"), state{PID: os.Getpid(), Endpoint: "http://127.0.0.1:1/mcp"}); err != nil {
		t.Fatal(err)
	}
	if stopped, err := Stop(dir, time.Second); err != nil || stopped {
		t.Fatalf("must not signal a foreign pid: %v %v", stopped, err)
	}
}

func TestStartRefusesChangedBinary(t *testing.T) {
	bin := copyBin(t, midnightBin(t))
	dir := t.TempDir()
	if _, err := Approve(dir, bin, filepath.Join(dir, "m.db")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(bin, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("tamper"))
	f.Close()
	if _, err := Start(dir); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("tampered binary must be refused, got %v", err)
	}
}
