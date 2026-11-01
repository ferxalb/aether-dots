package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResponsesSDKStreamingAndNoStorage(t *testing.T) {
	var seenAuth string
	var payload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/responses" {
			t.Errorf("request URL = %s", r.URL)
		}
		seenAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		response := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"A clear reply\\n\"}\n\n" +
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"with a useful next step.\\n\"}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(response)),
		}, nil
	})}
	g := Generator{Config: Config{Model: "gpt-6.1-sol", APIKey: "test-secret", HTTPClient: client}}
	var streamed strings.Builder
	got, err := g.Generate(context.Background(), "Dot objective:\nHelp me think through this.\n\nNewest user message:\nMake a brief", func(delta string) { streamed.WriteString(delta) })
	if err != nil {
		t.Fatal(err)
	}
	if got != "A clear reply\nwith a useful next step.\n" || streamed.String() != got {
		t.Fatalf("got %q streamed %q", got, streamed.String())
	}
	if seenAuth != "Bearer test-secret" {
		t.Fatalf("authorization = %q", seenAuth)
	}
	if payload["model"] != "gpt-6.1-sol" || payload["stream"] != true || payload["store"] != false {
		t.Fatalf("unexpected Responses request fields: %#v", payload)
	}
	if payload["max_output_tokens"] != float64(DefaultMaxOutputTokens) ||
		payload["reasoning"].(map[string]any)["effort"] != DefaultReasoningEffort ||
		payload["text"].(map[string]any)["verbosity"] != DefaultTextVerbosity {
		t.Fatalf("model generation controls were not sent: %#v", payload)
	}
	if payload["instructions"] == nil || !strings.Contains(payload["instructions"].(string), "no access") || !strings.Contains(payload["input"].(string), "Dot objective") {
		t.Fatalf("missing task input/instructions: %#v", payload)
	}
	if strings.Contains(payload["input"].(string), "source documents") || strings.Contains(payload["input"].(string), "/Users/") {
		t.Fatalf("model request included local file context: %#v", payload["input"])
	}
}

func TestReadinessValidatesGenerationControls(t *testing.T) {
	g := Generator{Config: Config{Model: "gpt-6.1-sol", APIKey: "test-secret"}}
	if ready, reason := g.Readiness(); !ready {
		t.Fatalf("default generation controls should be valid: %q", reason)
	}
	g.Config.ReasoningEffort = "turbo"
	if ready, reason := g.Readiness(); ready || !strings.Contains(reason, "reasoning effort") {
		t.Fatalf("invalid reasoning effort readiness=%t reason=%q", ready, reason)
	}
	g.Config.ReasoningEffort = "medium"
	g.Config.MaxOutputTokens = 100
	if ready, reason := g.Readiness(); ready || !strings.Contains(reason, "output tokens") {
		t.Fatalf("invalid output limit readiness=%t reason=%q", ready, reason)
	}
}

func TestReadinessRequiresKeyAndModel(t *testing.T) {
	g := Generator{Config: Config{Model: "gpt-6.1-sol"}}
	if ready, reason := g.Readiness(); ready || !strings.Contains(reason, "API key") {
		t.Fatalf("readiness without key: %t %q", ready, reason)
	}
	g.Config.APIKey = "test-secret"
	g.Config.Model = ""
	if ready, reason := g.Readiness(); ready || !strings.Contains(reason, "model name") {
		t.Fatalf("readiness without model: %t %q", ready, reason)
	}
}
