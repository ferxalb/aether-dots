package main

import (
	"os"
	"strings"
	"testing"
)

func TestCLIHelpFlagListsCommands(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "cli-help-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	os.Stdout = output
	err = run([]string{"--help"})
	os.Stdout = originalStdout
	if err != nil {
		t.Fatalf("help returned an error: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "routines [list|create|update") || !strings.Contains(string(contents), "skills [list|create|update") {
		t.Fatalf("help output omitted automation commands:\n%s", contents)
	}
}
