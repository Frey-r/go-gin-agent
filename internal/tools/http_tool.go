package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

type HTTPExecutor struct {
	timeout     time.Duration
	maxBodySize int64
}

type HTTPConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Query   map[string]string `json:"query"`
	Body    interface{}       `json:"body,omitempty"`
}

func NewHTTPExecutor(timeoutSecs int, maxBodyBytes int64) *HTTPExecutor {
	return &HTTPExecutor{
		timeout:     time.Duration(timeoutSecs) * time.Second,
		maxBodySize: maxBodyBytes,
	}
}

func (e *HTTPExecutor) Execute(ctx context.Context, args string) (string, error) {
	var cfg HTTPConfig
	if err := json.Unmarshal([]byte(args), &cfg); err != nil {
		return "", fmt.Errorf("invalid config JSON: %w", err)
	}

	if cfg.URL == "" {
		return "", fmt.Errorf("URL is required")
	}

	if err := e.validateURL(cfg.URL); err != nil {
		return "", fmt.Errorf("URL validation failed: %w", err)
	}

	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = "GET"
	}

	validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}
	if !validMethods[method] {
		return "", fmt.Errorf("unsupported HTTP method: %s", method)
	}

	req, err := e.buildRequest(ctx, method, cfg)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: e.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, e.maxBodySize))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	return string(body), nil
}

func (e *HTTPExecutor) validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only HTTP/HTTPS schemes allowed, got: %s", parsed.Scheme)
	}

	host := strings.ToLower(parsed.Host)
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("localhost not allowed")
	}

	if strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("internal hostnames not allowed")
	}

	ip := parsed.Hostname()
	if isPrivateIP(ip) {
		return fmt.Errorf("private IP addresses not allowed")
	}

	return nil
}

func isPrivateIP(ip string) bool {
	if strings.HasPrefix(ip, "10.") {
		return true
	}
	if strings.HasPrefix(ip, "172.") {
		parts := strings.Split(ip, ".")
		if len(parts) >= 2 {
			if part, err := parseInt(parts[1]); err == nil && part >= 16 && part <= 31 {
				return true
			}
		}
	}
	if strings.HasPrefix(ip, "192.168.") {
		return true
	}
	if ip == "0.0.0.0" {
		return true
	}
	return false
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func (e *HTTPExecutor) buildRequest(ctx context.Context, method string, cfg HTTPConfig) (*http.Request, error) {
	urlWithQuery := cfg.URL
	if len(cfg.Query) > 0 {
		q := url.Values{}
		for k, v := range cfg.Query {
			q.Set(k, v)
		}
		if strings.Contains(cfg.URL, "?") {
			urlWithQuery = cfg.URL + "&" + q.Encode()
		} else {
			urlWithQuery = cfg.URL + "?" + q.Encode()
		}
	}

	var body io.Reader
	if cfg.Body != nil {
		jsonBody, err := json.Marshal(cfg.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		body = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlWithQuery, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

type ConfigurableHTTPExecutor struct {
	delegate  *HTTPExecutor
	perAgentCfg map[string]HTTPConfig
}

func NewConfigurableHTTPExecutor(timeoutSecs int, maxBodyBytes int64) *ConfigurableHTTPExecutor {
	return &ConfigurableHTTPExecutor{
		delegate: NewHTTPExecutor(timeoutSecs, maxBodyBytes),
	}
}

func (e *ConfigurableHTTPExecutor) Execute(ctx context.Context, toolName string, args string, agentCfg map[string]model.ToolDefinition) (string, error) {
	return e.delegate.Execute(ctx, args)
}

func (e *ConfigurableHTTPExecutor) ExecuteWithConfig(ctx context.Context, args string) (string, error) {
	return e.delegate.Execute(ctx, args)
}