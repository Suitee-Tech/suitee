package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func assertNonThinking(t *testing.T, body map[string]any) {
	t.Helper()
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Errorf("thinking = %#v, want disabled", body["thinking"])
	}
}

func TestNormalizeDeepseekModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "missing", want: DefaultDeepseekModel},
		{name: "blank", input: "  ", want: DefaultDeepseekModel},
		{name: "legacy chat", input: "deepseek-chat", want: DefaultDeepseekModel},
		{name: "legacy reasoner", input: "deepseek-reasoner", want: DefaultDeepseekModel},
		{name: "supported override", input: " deepseek-v4-pro ", want: "deepseek-v4-pro"},
		{name: "compatible gateway override", input: "custom-model", want: "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeDeepseekModel(tt.input); got != tt.want {
				t.Fatalf("NormalizeDeepseekModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeepseekContract_ChatRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Errorf("request = %s %s, want POST /chat/completions", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sandbox-key" {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != DefaultDeepseekModel || body["tool_choice"] != "auto" || body["max_tokens"] != float64(256) {
			t.Errorf("unexpected chat contract: %#v", body)
		}
		assertNonThinking(t, body)
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Errorf("tools = %#v", body["tools"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{
          "choices":[{"message":{"role":"assistant","content":"sandbox ok"}}],
          "usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}
        }`)
	}))
	defer server.Close()

	client := NewDeepseekClient("sandbox-key", server.URL, "deepseek-chat", time.Second)
	message, usage, err := client.Chat(context.Background(), []DeepseekMessage{{Role: "user", Content: "ping"}}, []DeepseekTool{{
		Type: "function",
		Function: DeepseekToolFunction{
			Name:       "health_check",
			Parameters: map[string]any{"type": "object"},
		},
	}}, 256)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if message.Role != "assistant" || message.Content != "sandbox ok" {
		t.Fatalf("unexpected message: %#v", message)
	}
	if usage.TotalTokens != 14 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestDeepseekContract_JSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		responseFormat, _ := body["response_format"].(map[string]any)
		if responseFormat["type"] != "json_object" || body["tool_choice"] != "none" {
			t.Errorf("unexpected JSON-mode contract: %#v", body)
		}
		if body["model"] != "deepseek-v4-pro" {
			t.Errorf("model = %#v, want explicit deepseek-v4-pro", body["model"])
		}
		assertNonThinking(t, body)
		if _, hasTools := body["tools"]; hasTools {
			t.Errorf("JSON mode must not send tools: %#v", body["tools"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}],"usage":{"total_tokens":8}}`)
	}))
	defer server.Close()

	client := NewDeepseekClient("sandbox-key", server.URL, " deepseek-v4-pro ", time.Second)
	message, _, err := client.ChatJSON(context.Background(), []DeepseekMessage{{Role: "user", Content: "extract"}}, 128)
	if err != nil {
		t.Fatalf("ChatJSON: %v", err)
	}
	if message.Content != `{"ok":true}` {
		t.Fatalf("unexpected JSON content: %q", message.Content)
	}
}

func TestDeepseekContract_StreamAndTimeout(t *testing.T) {
	t.Run("stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["stream"] != true {
				t.Errorf("stream flag = %#v", body["stream"])
			}
			assertNonThinking(t, body)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Olá "}}]}`)
			_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"sandbox"}}],"usage":{"total_tokens":5}}`)
			_, _ = fmt.Fprintln(w, `data: [DONE]`)
		}))
		defer server.Close()

		client := NewDeepseekClient("sandbox-key", server.URL, DefaultDeepseekModel, time.Second)
		var deltas string
		message, usage, err := client.ChatStream(context.Background(), []DeepseekMessage{{Role: "user", Content: "ping"}}, nil, 64, func(text string) {
			deltas += text
		})
		if err != nil {
			t.Fatalf("ChatStream: %v", err)
		}
		if message.Content != "Olá sandbox" || deltas != message.Content || usage.TotalTokens != 5 {
			t.Fatalf("unexpected stream result: message=%#v deltas=%q usage=%#v", message, deltas, usage)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()

		client := NewDeepseekClient("sandbox-key", server.URL, DefaultDeepseekModel, 10*time.Millisecond)
		started := time.Now()
		_, _, err := client.Chat(context.Background(), []DeepseekMessage{{Role: "user", Content: "ping"}}, nil, 16)
		if err == nil {
			t.Fatal("expected timeout")
		}
		if elapsed := time.Since(started); elapsed >= time.Second {
			t.Fatalf("configured timeout was not enforced: elapsed=%s", elapsed)
		}
	})
}
