//go:build !linux

package midnightsup

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// command runs the binary by path. This platform cannot exec a descriptor, so
// the path is re-hashed immediately before launch by the caller; a swap in
// that window is a residual risk. Not validated on macOS yet.
func command(_ *os.File, path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}

// processIdentity uses ps(1) because there is no /proc. Arguments containing
// spaces cannot be told apart, which only makes matching stricter.
func processIdentity(pid int) (string, []string, bool) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return "", nil, false
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", nil, false
	}
	return fields[0], fields, true
}
