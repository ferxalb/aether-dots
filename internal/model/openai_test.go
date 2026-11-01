package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aether-dots/internal/source"
)

func TestChatCompletionsStreaming(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"# Decision brief\\n\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Evidence: notes.md\\n\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	g := Generator{Config: Config{BaseURL: server.URL + "/v1", Model: "test-model", API: "chat", APIKey: "test-secret", KeyEnv: "TEST_KEY"}}
	var streamed strings.Builder
	got, err := g.Generate(context.Background(), "Make a brief", []source.Document{{Path: "notes.md", Text: "TODO: choose"}}, func(delta string) { streamed.WriteString(delta) })
	if err != nil {
		t.Fatal(err)
	}
	if got != "# Decision brief\nEvidence: notes.md\n" || streamed.String() != got {
		t.Fatalf("got %q streamed %q", got, streamed.String())
	}
	if seenAuth != "Bearer test-secret" {
		t.Fatalf("authorization = %q", seenAuth)
	}
}

func TestResponsesStreamingAndRequestDoesNotStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "gpt-6.1-sol" || body["stream"] != true || body["store"] != false {
			t.Errorf("unexpected Responses request fields: %#v", body)
		}
		if body["instructions"] == nil || !strings.Contains(body["input"].(string), "selected source documents") {
			t.Errorf("missing task input/instructions: %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"AETHER_OK\\n\"}\n\n")
		_, _ = fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
	}))
	defer server.Close()
	g := Generator{Config: Config{BaseURL: server.URL + "/v1", Model: "gpt-6.1-sol", API: "responses", APIKey: "test-secret", KeyEnv: "TEST_KEY"}}
	if err := g.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessAndRequestURL(t *testing.T) {
	g := Generator{Config: Config{KeyEnv: "OPENAI_API_KEY", Model: "test", BaseURL: "https://example.test/v1", API: "responses"}}
	if ready, reason := g.Readiness(); ready || !strings.Contains(reason, "OPENAI_API_KEY") {
		t.Fatalf("readiness: %t %q", ready, reason)
	}
	for _, test := range []struct{ base, api, want string }{
		{"https://example.test", "responses", "https://example.test/v1/responses"},
		{"https://example.test/v1/", "responses", "https://example.test/v1/responses"},
		{"https://example.test/v1/responses", "responses", "https://example.test/v1/responses"},
		{"http://localhost:8000/v1", "chat", "http://localhost:8000/v1/chat/completions"},
	} {
		got, err := requestURL(test.base, test.api)
		if err != nil || got != test.want {
			t.Fatalf("requestURL(%q,%q) = %q, %v", test.base, test.api, got, err)
		}
	}
	for _, base := range []string{"file:///tmp/model", "https://user:token@example.test/v1", "https://example.test/v1?key=secret"} {
		if _, err := requestURL(base, "responses"); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", base)
		}
	}
	g.Config.BaseURL = "https://user:secret@example.test/v1?token=secret"
	if label := g.Config.EndpointLabel(); label != "https://example.test" {
		t.Fatalf("endpoint label leaked URL data: %q", label)
	}
}
