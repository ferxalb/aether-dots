package midnightsup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeMidnight writes a shell stand-in that identifies as Midnight, records
// its pid, prints the given listening line for serve-http and then idles.
func fakeMidnight(t *testing.T, listen string) (bin, pidFile string) {
	t.Helper()
	dir := t.TempDir()
	pidFile = filepath.Join(dir, "pid")
	bin = filepath.Join(dir, "aether-midnight")
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
*--version*) echo "aether-midnight 0.0-test";;
*) echo $$ > %s
   [ -n "%s" ] && echo "%s"
   [ -n "%s" ] || exit 3
   exec sleep 60;;
esac
`, pidFile, listen, listen, listen)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, pidFile
}

func gone(t *testing.T, pidFile string) bool {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return true // never started
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	for i := 0; i < 100; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && strings.Contains(string(b), ") Z") {
			return true // reaped by init shortly; a zombie is not running
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestPartialStartLeavesNothingRunning(t *testing.T) {
	bin, pidFile := fakeMidnight(t, "listening http://127.0.0.1:45999/mcp")
	dir := t.TempDir()
	if _, err := Approve(dir, bin, filepath.Join(dir, "m.db")); err != nil {
		t.Fatal(err)
	}
	// Persisting the discovered port fails: a directory squats on the temp name.
	if err := os.Mkdir(filepath.Join(root(dir), "approved.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(dir); err == nil {
		t.Fatal("start must fail when the port cannot be persisted")
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatal("test is vacuous: the fake server never ran")
	}
	if !gone(t, pidFile) {
		t.Fatal("child survived a failed start")
	}
	if _, err := os.Stat(filepath.Join(root(dir), "server.json")); err == nil {
		t.Fatal("state file left behind by a failed start")
	}
	// State-file write failure takes the same cleanup path.
	_ = os.Remove(filepath.Join(root(dir), "approved.json.tmp"))
	if err := os.Mkdir(filepath.Join(root(dir), "server.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(dir); err == nil {
		t.Fatal("start must fail when state cannot be written")
	}
	if !gone(t, pidFile) {
		t.Fatal("child survived a failed state write")
	}
}

func TestStartRejectsNonLoopbackAndEarlyExit(t *testing.T) {
	for name, listen := range map[string]string{"non-loopback": "listening http://0.0.0.0:45998/mcp", "early exit": ""} {
		bin, pidFile := fakeMidnight(t, listen)
		dir := t.TempDir()
		if _, err := Approve(dir, bin, filepath.Join(dir, "m.db")); err != nil {
			t.Fatal(err)
		}
		if _, err := Start(dir); err == nil {
			t.Fatalf("%s: start must fail", name)
		}
		if !gone(t, pidFile) {
			t.Fatalf("%s: child survived", name)
		}
		if _, err := os.Stat(filepath.Join(root(dir), "server.json")); err == nil {
			t.Fatalf("%s: state file left behind", name)
		}
	}
}

func TestCorruptStateIsReportedNotRepaired(t *testing.T) {
	bin, _ := fakeMidnight(t, "")
	dir := t.TempDir()
	if _, err := Approve(dir, bin, filepath.Join(dir, "m.db")); err != nil {
		t.Fatal(err)
	}
	r := root(dir)
	approved := filepath.Join(r, "approved.json")
	for name, content := range map[string]string{
		"garbage":       "{not json",
		"relative path": `{"binary":"x","sha256":"` + strings.Repeat("a", 64) + `","db":"/d"}`,
		"bad hash":      `{"binary":"/b","sha256":"zz","db":"/d"}`,
		"bad port":      `{"binary":"/b","sha256":"` + strings.Repeat("a", 64) + `","db":"/d","port":70000}`,
	} {
		if err := os.WriteFile(approved, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Get(dir); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: Get err = %v", name, err)
		}
		if _, err := Start(dir); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: Start err = %v", name, err)
		}
		if _, err := Stop(dir, time.Second); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: Stop err = %v", name, err)
		}
		if raw, _ := os.ReadFile(approved); string(raw) != content {
			t.Errorf("%s: corrupt config was modified", name)
		}
	}
	// Corrupt server.json is reported and kept: it may be the only record of a running server.
	if _, err := Approve(dir, bin, filepath.Join(dir, "m.db")); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(r, "server.json")
	for _, content := range []string{"nope", `{"pid":0,"endpoint":"http://127.0.0.1:1/mcp"}`, `{"pid":4242,"endpoint":"ftp://x"}`} {
		if err := os.WriteFile(statePath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Get(dir); !errors.Is(err, ErrCorrupt) {
			t.Errorf("state %q: Get err = %v", content, err)
		}
		if _, err := Start(dir); !errors.Is(err, ErrCorrupt) {
			t.Errorf("state %q: Start err = %v", content, err)
		}
		if _, err := Stop(dir, time.Second); !errors.Is(err, ErrCorrupt) {
			t.Errorf("state %q: Stop err = %v", content, err)
		}
		if raw, _ := os.ReadFile(statePath); string(raw) != content {
			t.Errorf("state %q was modified", content)
		}
	}
}
