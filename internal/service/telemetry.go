package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// TelemetryService sends trace data to Langfuse asynchronously.
type TelemetryService struct {
	publicKey  string
	secretKey  string
	host       string
	httpClient *http.Client
	enabled    bool
}

// NewTelemetryService creates a new TelemetryService.
// If publicKey or secretKey are empty, telemetry is disabled silently.
func NewTelemetryService(publicKey, secretKey, host string) *TelemetryService {
	enabled := publicKey != "" && secretKey != ""
	return &TelemetryService{
		publicKey: publicKey,
		secretKey: secretKey,
		host:      host,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		enabled: enabled,
	}
}

// TraceData contains the information to report to Langfuse.
type TraceData struct {
	TenantID     string
	UserID       string
	ThreadID     string
	Model        string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	ToolCalls    []string
}

// ReportAsync fires a goroutine to send trace data to Langfuse.
// This adds 0ms latency to the user's request.
func (t *TelemetryService) ReportAsync(data TraceData) {
	if !t.enabled {
		return
	}

	go func() {
		if err := t.report(data); err != nil {
			log.Error().Err(err).
				Str("tenant_id", data.TenantID).
				Str("thread_id", data.ThreadID).
				Msg("langfuse: failed to report trace")
		}
	}()
}

func (t *TelemetryService) report(data TraceData) error {
	payload := map[string]interface{}{
		"batch": []map[string]interface{}{
			{
				"type":      "trace-create",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"body": map[string]interface{}{
					"name":   "chat-completion",
					"userId": data.UserID,
					"metadata": map[string]interface{}{
						"tenant_id":    data.TenantID,
						"thread_id":    data.ThreadID,
						"model":        data.Model,
						"input_tokens": data.InputTokens,
						"output_tokens": data.OutputTokens,
						"tool_calls":   data.ToolCalls,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, t.host+"/api/public/ingestion", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(t.publicKey, t.secretKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Warn().Int("status", resp.StatusCode).Msg("langfuse: non-OK response")
	}

	return nil
}
