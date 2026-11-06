//go:build linux

package midnightsup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// command runs the already-hashed descriptor itself, so the bytes that were
// verified are the bytes that execute (no path swap between check and exec).
func command(f *os.File, _ string, args ...string) *exec.Cmd {
	cmd := exec.Command("/proc/self/fd/3", args...)
	cmd.Args[0] = "aether-midnight"
	cmd.ExtraFiles = []*os.File{f}
	return cmd
}

// processIdentity returns the executable path and argv of a live process.
func processIdentity(pid int) (string, []string, bool) {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", nil, false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", nil, false
	}
	return strings.TrimSuffix(exe, " (deleted)"), strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00"), true
}
