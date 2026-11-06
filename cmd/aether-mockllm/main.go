// Command aether-mockllm is a deterministic, offline OpenAI-compatible chat
// server for reproducible tests and benchmarks. It never contacts the network.
//
// Behaviour: if the Dot offers a Midnight recall/remember tool it calls it with
// the Dot's declared memory scope, then answers using only the tool result.
//
//	"remember: <text>"  -> calls remember
//	anything else       -> calls recall with the message as the query
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var scopeRE = regexp.MustCompile(`Midnight memory scope: (dot:[A-Za-z0-9_-]+)`)

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
}

func (m message) text() string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(m.Content, &parts)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "loopback listen address")
	delay := flag.Duration("delay", 0, "artificial model latency before the first token")
	flag.Parse()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
	}
	if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() {
		panic("mock model only binds loopback")
	}
	fmt.Printf("listening http://%s/v1\n", listener.Addr())
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []message `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		time.Sleep(*delay)
		var prompt, toolResult string
		for _, m := range req.Messages {
			switch m.Role {
			case "user":
				prompt = m.text()
			case "tool":
				toolResult = m.text()
			}
		}
		newest := prompt
		if i := strings.LastIndex(prompt, "Newest user message:\n"); i >= 0 {
			newest = prompt[i+len("Newest user message:\n"):]
		}
		newest = strings.TrimSpace(newest)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		send := func(delta map[string]any, finish any) {
			chunk := map[string]any{"id": "mock", "object": "chat.completion.chunk", "model": "mock",
				"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if flusher != nil {
				flusher.Flush()
			}
		}
		find := func(suffix string) string {
			for _, t := range req.Tools {
				if strings.Contains(t.Function.Name, "_"+suffix+"_") {
					return t.Function.Name
				}
			}
			return ""
		}
		scope := ""
		if m := scopeRE.FindStringSubmatch(prompt); m != nil {
			scope = m[1]
		}
		if toolResult == "" && scope != "" {
			tool, args := find("recall"), map[string]any{"scope": scope, "query": newest, "limit": 3}
			if rest, ok := strings.CutPrefix(newest, "remember:"); ok && find("remember") != "" {
				tool, args = find("remember"), map[string]any{"scope": scope, "kind": "source", "content": strings.TrimSpace(rest)}
			}
			if tool != "" {
				raw, _ := json.Marshal(args)
				send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
					"index": 0, "id": "call_mock_1", "type": "function",
					"function": map[string]any{"name": tool, "arguments": string(raw)}}}}, nil)
				send(map[string]any{}, "tool_calls")
				fmt.Fprint(w, "data: [DONE]\n\n")
				return
			}
		}
		answer := "Mock reply (no Midnight tools were granted to this Dot)."
		if toolResult != "" {
			answer = "Grounded in Midnight: " + clip(toolResult, 1200)
		}
		for _, word := range strings.SplitAfter(answer, " ") {
			send(map[string]any{"content": word}, nil)
		}
		send(map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	_ = http.Serve(listener, mux)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
