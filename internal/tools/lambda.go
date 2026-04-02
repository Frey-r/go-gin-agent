package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LambdaExecutor invokes an AWS Lambda function via API Gateway HTTP endpoint.
type LambdaExecutor struct {
	endpoint   string // API Gateway URL
	httpClient *http.Client
}

// NewLambdaExecutor creates a Lambda tool executor.
func NewLambdaExecutor(endpoint string, timeoutSeconds int) *LambdaExecutor {
	return &LambdaExecutor{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// Execute calls the Lambda via HTTP POST with the arguments JSON as body.
func (l *LambdaExecutor) Execute(ctx context.Context, arguments string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint, bytes.NewBufferString(arguments))
	if err != nil {
		return "", fmt.Errorf("lambda: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("lambda: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("lambda: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lambda: error %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
