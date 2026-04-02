package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GrokProvider implements Provider for xAI's Grok API.
// Grok uses an OpenAI-compatible API format.
type GrokProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewGrokProvider creates a Grok provider.
func NewGrokProvider(apiKey string, timeoutSeconds int) *GrokProvider {
	return &GrokProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		baseURL: "https://api.x.ai/v1",
	}
}

func (g *GrokProvider) Name() string { return "grok" }

// ChatStream sends a streaming request to the Grok API (OpenAI-compatible).
func (g *GrokProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)

	// Build OpenAI-compatible request
	reqBody := g.buildRequest(messages, tools)
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal request: %w", err)
	}

	url := g.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("grok: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("grok: API error %d: %s", resp.StatusCode, string(body))
	}

	go g.streamResponse(ctx, resp.Body, ch)

	return ch, nil
}

func (g *GrokProvider) streamResponse(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: EventError, Error: "context cancelled"}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk grokStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Handle tool calls
			if len(delta.ToolCalls) > 0 {
				toolCalls := make([]ToolCall, 0, len(delta.ToolCalls))
				for _, tc := range delta.ToolCalls {
					toolCalls = append(toolCalls, ToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
				ch <- StreamEvent{
					Type:      EventToolCall,
					ToolCalls: toolCalls,
				}
			}

			// Handle text content
			if delta.Content != "" {
				ch <- StreamEvent{
					Type:    EventToken,
					Content: delta.Content,
				}
			}
		}

		// Usage info (sent in the final chunk)
		if chunk.Usage != nil {
			ch <- StreamEvent{
				Type:         EventDone,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				Model:        chunk.Model,
			}
			return
		}
	}

	// If we didn't get usage info, still send done
	ch <- StreamEvent{Type: EventDone, Model: "grok"}
}

func (g *GrokProvider) buildRequest(messages []Message, tools []ToolDef) map[string]interface{} {
	msgs := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs = append(msgs, msg)
	}

	req := map[string]interface{}{
		"model":    "grok-3",
		"messages": msgs,
		"stream":   true,
		"stream_options": map[string]interface{}{
			"include_usage": true,
		},
	}

	if len(tools) > 0 {
		toolDefs := make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			toolDefs = append(toolDefs, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		req["tools"] = toolDefs
	}

	return req
}

// Grok API response structures (OpenAI-compatible)
type grokStreamChunk struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Choices []grokStreamChoice `json:"choices"`
	Usage   *grokUsage        `json:"usage,omitempty"`
}

type grokStreamChoice struct {
	Delta grokDelta `json:"delta"`
}

type grokDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []grokToolCall  `json:"tool_calls,omitempty"`
}

type grokToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function grokFunction `json:"function"`
}

type grokFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type grokUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
