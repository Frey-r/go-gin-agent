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

// GeminiProvider implements Provider for Google's Gemini API.
type GeminiProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewGeminiProvider creates a Gemini provider with sensible defaults.
func NewGeminiProvider(apiKey string, timeoutSeconds int) *GeminiProvider {
	return &GeminiProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}
}

func (g *GeminiProvider) Name() string { return "gemini" }

// ChatStream sends a streaming request to the Gemini API and returns events via channel.
func (g *GeminiProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)

	// Build Gemini request body
	reqBody := g.buildRequest(messages, tools)
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	// Determine model from context or default
	model := "gemini-2.5-pro"
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse",
		g.baseURL, model, g.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: API error %d: %s", resp.StatusCode, string(body))
	}

	// Stream the response in a goroutine
	go g.streamResponse(ctx, resp.Body, ch)

	return ch, nil
}

func (g *GeminiProvider) streamResponse(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	var totalInput, totalOutput int

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

		var geminiResp geminiStreamResponse
		if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
			continue
		}

		// Track usage
		if geminiResp.UsageMetadata != nil {
			totalInput = geminiResp.UsageMetadata.PromptTokenCount
			totalOutput = geminiResp.UsageMetadata.CandidatesTokenCount
		}

		for _, candidate := range geminiResp.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					ch <- StreamEvent{
						Type: EventToolCall,
						ToolCalls: []ToolCall{{
							ID:        part.FunctionCall.Name, // Gemini uses name as ID
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						}},
					}
				} else if part.Text != "" {
					ch <- StreamEvent{
						Type:    EventToken,
						Content: part.Text,
					}
				}
			}
		}
	}

	ch <- StreamEvent{
		Type:         EventDone,
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
		Model:        "gemini-2.5-pro",
	}
}

func (g *GeminiProvider) buildRequest(messages []Message, tools []ToolDef) map[string]interface{} {
	contents := make([]map[string]interface{}, 0, len(messages))

	var systemInstruction string
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemInstruction = msg.Content
			continue
		}

		role := msg.Role
		if role == RoleAssistant {
			role = "model"
		}

		contents = append(contents, map[string]interface{}{
			"role": role,
			"parts": []map[string]interface{}{
				{"text": msg.Content},
			},
		})
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	if systemInstruction != "" {
		req["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemInstruction},
			},
		}
	}

	if len(tools) > 0 {
		funcDecls := make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			funcDecls = append(funcDecls, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			})
		}
		req["tools"] = []map[string]interface{}{
			{"functionDeclarations": funcDecls},
		}
	}

	return req
}

// Gemini API response structures
type geminiStreamResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type geminiPart struct {
	Text         string              `json:"text,omitempty"`
	FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
