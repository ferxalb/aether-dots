package task

import (
	"strings"
	"testing"
)

func TestAgentTemplatesFitLimits(t *testing.T) {
	for _, tmpl := range AgentTemplates {
		if len(tmpl.Name) > MaxConversationNameBytes || len(tmpl.Purpose) > MaxConversationPurposeBytes ||
			len(tmpl.Instructions) > MaxConversationInstructionsBytes || len(tmpl.SuggestedRoutinePrompt) > MaxRoutinePromptBytes {
			t.Fatalf("template %q exceeds a profile limit", tmpl.ID)
		}
		if got, ok := FindAgentTemplate(tmpl.ID); !ok || got.Name != tmpl.Name {
			t.Fatalf("FindAgentTemplate(%q) = %v, %v", tmpl.ID, got, ok)
		}
	}
	if !strings.Contains(TriageTemplate.Instructions, "aether_github_search_issues") || !strings.Contains(TriageTemplate.Instructions, "untrusted") {
		t.Fatal("triage instructions lost the duplicate search or the untrusted-text rule")
	}
	if tmpl, ok := TemplateForInstructions(TriageTemplate.Instructions); !ok || tmpl.ID != "triage" {
		t.Fatal("triage instructions not recognized")
	}
	if _, ok := TemplateForInstructions(TriageTemplate.Instructions + " edited"); ok {
		t.Fatal("edited instructions still matched the template")
	}
	if TriageTemplate.SuggestedRoutineName == "" || len(TriageTemplate.SuggestedRoutineName) > MaxRoutineNameBytes ||
		TriageTemplate.SuggestedRoutineIntervalSeconds != 24*60*60 {
		t.Fatal("triage routine suggestion is not a valid daily routine")
	}
	if _, ok := FindAgentTemplate("nope"); ok {
		t.Fatal("found an unknown template")
	}
}
