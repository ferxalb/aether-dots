package main

import (
	"os"
	"os/exec"
	"testing"
)

// TestMidnightEndToEnd runs the real CLI against the real aether-midnight
// binary and the offline mock model. Set MIDNIGHT_BIN to enable it.
func TestMidnightEndToEnd(t *testing.T) {
	bin := os.Getenv("MIDNIGHT_BIN")
	if bin == "" {
		t.Skip("MIDNIGHT_BIN not set")
	}
	out, err := exec.Command("../../scripts/e2e-midnight.sh").CombinedOutput()
	if err != nil {
		t.Fatalf("end-to-end flow failed: %v\n%s", err, out)
	}
}

func TestSplitFlagsAllowsFlagsAfterPositionals(t *testing.T) {
	pos, flags := splitFlags([]string{"dot", "--message", "hello world", "--accept-model-disclosure"})
	if len(pos) != 1 || pos[0] != "dot" || len(flags) != 3 {
		t.Fatalf("pos=%v flags=%v", pos, flags)
	}
}
