package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
)

const DefaultDeepseekModel = "deepseek-v4-flash"

// NormalizeDeepseekModel translates retired official aliases while preserving
// explicit model names for OpenAI-compatible gateways.
func NormalizeDeepseekModel(model string) string {
	switch model = strings.TrimSpace(model); model {
	case "", "deepseek-chat", "deepseek-reasoner":
		return DefaultDeepseekModel
	default:
		return model
	}
}

type DeepseekTool struct {
	Type     string               `json:"type"`
	Function DeepseekToolFunction `json:"function"`
	ReadOnly bool                 `json:"-"`
	Domain   string               `json:"-"`
	MinRole  string               `json:"-"`
}

type DeepseekToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

type DeepseekMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCalls  []DeepseekToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	Name       string             `json:"name,omitempty"`
}

type DeepseekToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function DeepseekToolCallFunc `json:"function"`
}

type DeepseekToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type DeepseekChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type DeepseekClient struct {
	httpClient *clients.HTTPClient
	apiKey     string
	model      string
	thinking   *deepseekThinking
}

func NewDeepseekClient(apiKey, baseURL, model string, timeout time.Duration) *DeepseekClient {
	model = NormalizeDeepseekModel(model)
	var thinking *deepseekThinking
	if model == DefaultDeepseekModel || model == "deepseek-v4-pro" {
		// The existing chat loop does not replay reasoning_content across tool
		// calls, so keep the non-thinking behavior of the former deepseek-chat.
		thinking = &deepseekThinking{Type: "disabled"}
	}

	httpClient := clients.NewHTTPClient(strings.TrimRight(baseURL, "/"),
		clients.WithTimeout(timeout),
		clients.WithHeaders(func(h http.Header) {
			h.Set("Authorization", "Bearer "+apiKey)
		}),
	)
	return &DeepseekClient{
		httpClient: httpClient,
		apiKey:     apiKey,
		model:      model,
		thinking:   thinking,
	}
}

type deepseekThinking struct {
	Type string `json:"type"`
}

type deepseekChatRequest struct {
	Model          string                  `json:"model"`
	Messages       []DeepseekMessage       `json:"messages"`
	Tools          []DeepseekTool          `json:"tools,omitempty"`
	ToolChoice     string                  `json:"tool_choice,omitempty"`
	MaxTokens      int                     `json:"max_tokens,omitempty"`
	Temperature    float64                 `json:"temperature,omitempty"`
	Stream         bool                    `json:"stream,omitempty"`
	ResponseFormat *DeepseekResponseFormat `json:"response_format,omitempty"`
	Thinking       *deepseekThinking       `json:"thinking,omitempty"`
}

// DeepseekResponseFormat ativa o JSON mode da API (compatível com o schema OpenAI).
// Com Type="json_object" a resposta é garantidamente JSON válido — mas não o schema
// correto, então os validadores em document_extraction_service.go são o gate real.
type DeepseekResponseFormat struct {
	Type string `json:"type"`
}

type deepseekChatResponse struct {
	Choices []struct {
		Message DeepseekMessage `json:"message"`
	} `json:"choices"`
	Usage DeepseekChatUsage `json:"usage"`
}

type deepseekToolCallDelta struct {
	Index    int    `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type deepseekChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string                  `json:"content,omitempty"`
			ToolCalls []deepseekToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *DeepseekChatUsage `json:"usage,omitempty"`
}

func (c *DeepseekClient) Chat(ctx context.Context, messages []DeepseekMessage, tools []DeepseekTool, maxTokens int) (DeepseekMessage, DeepseekChatUsage, error) {
	req := deepseekChatRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  "auto",
		MaxTokens:   maxTokens,
		Temperature: 0.2,
		Stream:      false,
		Thinking:    c.thinking,
	}

	resp, err := c.httpClient.Do(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek error status=%d body=%s", resp.StatusCode, msg)
	}

	var out deepseekChatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to parse deepseek response: %w", err)
	}
	if len(out.Choices) == 0 {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek returned no choices")
	}
	return out.Choices[0].Message, out.Usage, nil
}

// doBlocking executa uma requisição não-streaming e devolve a primeira choice.
// Compartilhado por ChatJSON (ver Chat/ChatNoTools, que mantêm o corpo próprio).
func (c *DeepseekClient) doBlocking(ctx context.Context, req deepseekChatRequest) (DeepseekMessage, DeepseekChatUsage, error) {
	resp, err := c.httpClient.Do(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek error status=%d body=%s", resp.StatusCode, msg)
	}

	var out deepseekChatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to parse deepseek response: %w", err)
	}
	if len(out.Choices) == 0 {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek returned no choices")
	}
	return out.Choices[0].Message, out.Usage, nil
}

// ChatJSON faz uma chamada bloqueante com JSON mode ligado e sem ferramentas.
// Usado pela extração de documentos (saída estruturada estrita).
func (c *DeepseekClient) ChatJSON(ctx context.Context, messages []DeepseekMessage, maxTokens int) (DeepseekMessage, DeepseekChatUsage, error) {
	return c.doBlocking(ctx, deepseekChatRequest{
		Model:          c.model,
		Messages:       messages,
		ToolChoice:     "none",
		MaxTokens:      maxTokens,
		Temperature:    0.2,
		ResponseFormat: &DeepseekResponseFormat{Type: "json_object"},
		Thinking:       c.thinking,
	})
}

func (c *DeepseekClient) ChatStream(
	ctx context.Context,
	messages []DeepseekMessage,
	tools []DeepseekTool,
	maxTokens int,
	onDelta func(text string),
) (DeepseekMessage, DeepseekChatUsage, error) {
	req := deepseekChatRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  "auto",
		MaxTokens:   maxTokens,
		Temperature: 0.2,
		Stream:      true,
		Thinking:    c.thinking,
	}

	resp, err := c.httpClient.Do(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 32_000))
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek error status=%d body=%s", resp.StatusCode, msg)
	}

	var content strings.Builder
	toolCalls := make([]DeepseekToolCall, 0)
	usage := DeepseekChatUsage{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk deepseekChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		if d.Content != "" {
			content.WriteString(d.Content)
			if onDelta != nil {
				onDelta(d.Content)
			}
		}
		if len(d.ToolCalls) > 0 {
			for _, tc := range d.ToolCalls {
				if tc.Index < 0 {
					continue
				}
				for len(toolCalls) <= tc.Index {
					toolCalls = append(toolCalls, DeepseekToolCall{
						ID:   "",
						Type: "function",
						Function: DeepseekToolCallFunc{
							Name:      "",
							Arguments: "",
						},
					})
				}
				existing := toolCalls[tc.Index]
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Type != "" {
					existing.Type = tc.Type
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					existing.Function.Arguments += tc.Function.Arguments
				}
				toolCalls[tc.Index] = existing
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, err
	}

	return DeepseekMessage{
		Role:      "assistant",
		Content:   content.String(),
		ToolCalls: toolCalls,
	}, usage, nil
}

func (c *DeepseekClient) ChatNoTools(ctx context.Context, messages []DeepseekMessage, maxTokens int) (DeepseekMessage, DeepseekChatUsage, error) {
	req := deepseekChatRequest{
		Model:       c.model,
		Messages:    messages,
		ToolChoice:  "none",
		MaxTokens:   maxTokens,
		Temperature: 0.2,
		Stream:      false,
		Thinking:    c.thinking,
	}

	resp, err := c.httpClient.Do(ctx, http.MethodPost, "/chat/completions", req)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek error status=%d body=%s", resp.StatusCode, msg)
	}

	var out deepseekChatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("failed to parse deepseek response: %w", err)
	}
	if len(out.Choices) == 0 {
		return DeepseekMessage{}, DeepseekChatUsage{}, fmt.Errorf("deepseek returned no choices")
	}
	return out.Choices[0].Message, out.Usage, nil
}
