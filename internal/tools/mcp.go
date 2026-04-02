package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MCPClient implements the Model Context Protocol client for querying
// the knowledge base via Cloudflare Workers.
type MCPClient struct {
	endpoint   string
	httpClient *http.Client
}

// NewMCPClient creates an MCP client pointing at the given Cloudflare Worker URL.
func NewMCPClient(endpoint string, timeoutSeconds int) *MCPClient {
	return &MCPClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// Execute sends a query to the MCP server and returns the context it retrieves.
func (m *MCPClient) Execute(ctx context.Context, arguments string) (string, error) {
	payload := map[string]interface{}{
		"query": arguments,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("mcp: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("mcp: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("mcp: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("mcp: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mcp: error %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
