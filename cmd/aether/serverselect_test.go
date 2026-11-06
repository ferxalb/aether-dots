package main

import (
	"strings"
	"testing"

	"aether-dots/internal/task"
)

func srv(id, name, endpoint string) task.MCPServer {
	return task.MCPServer{ID: id, Name: name, Endpoint: endpoint,
		Tools: []task.MCPTool{{Name: "recall", Fingerprint: "fp-" + id}}}
}

func dotWith(id string, serverIDs ...string) task.Conversation {
	c := task.Conversation{ID: id}
	for _, s := range serverIDs {
		c.MCPToolGrants = append(c.MCPToolGrants, task.MCPToolGrant{ServerID: s, ToolName: "recall", Fingerprint: "fp-" + s})
	}
	return c
}

func TestMidnightServerSelection(t *testing.T) {
	own := srv("s1", "Midnight dot-a", "http://127.0.0.1:1/mcp/dot-a")
	otherDot := srv("s2", "Midnight dot-b", "http://127.0.0.1:1/mcp/dot-b")
	manualA := srv("s3", "Team memory", "http://127.0.0.1:2/mcp")
	manualB := srv("s4", "Other memory", "http://127.0.0.1:3/mcp")
	cases := []struct {
		name    string
		dot     task.Conversation
		servers []task.MCPServer
		want    string
		errHas  string
	}{
		{"own server preferred over manual", dotWith("a", "s3", "s1"), []task.MCPServer{own, manualA}, "s1", ""},
		{"single manual server still works", dotWith("a", "s3"), []task.MCPServer{manualA}, "s3", ""},
		{"two manual servers are ambiguous", dotWith("a", "s3", "s4"), []task.MCPServer{manualA, manualB}, "", "ambiguous"},
		{"another Dot's record is refused", dotWith("a", "s2"), []task.MCPServer{otherDot}, "", "different Dot"},
		{"no grant", dotWith("a"), []task.MCPServer{own}, "", "not granted"},
		{"grant for a removed server", dotWith("a", "gone"), []task.MCPServer{own}, "", "not granted"},
	}
	for _, tc := range cases {
		got, err := midnightServerFor(tc.dot, tc.servers, "recall")
		if tc.errHas != "" {
			if err == nil || !strings.Contains(err.Error(), tc.errHas) {
				t.Errorf("%s: err=%v, want %q", tc.name, err, tc.errHas)
			}
			continue
		}
		if err != nil || got.ID != tc.want {
			t.Errorf("%s: got %q err=%v, want %q", tc.name, got.ID, err, tc.want)
		}
	}
	// A server whose tool fingerprint changed after the grant is stale.
	drift := own
	drift.Tools = []task.MCPTool{{Name: "recall", Fingerprint: "changed"}}
	if _, err := midnightServerFor(dotWith("a", "s1"), []task.MCPServer{drift}, "recall"); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Errorf("fingerprint drift must be refused, got %v", err)
	}
	// Tool not covered by any grant.
	if _, err := midnightServerFor(dotWith("a", "s1"), []task.MCPServer{own}, "forget"); err == nil {
		t.Error("ungranted tool must be refused")
	}
}
