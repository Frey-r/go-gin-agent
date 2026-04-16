package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/service"
)

// ChatHandler handles the streaming chat endpoint.
type ChatHandler struct {
	orchestrator *service.Orchestrator
	validate     *validator.Validate
	timeout      time.Duration
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(orchestrator *service.Orchestrator, timeoutSeconds int) *ChatHandler {
	return &ChatHandler{
		orchestrator: orchestrator,
		validate:     validator.New(),
		timeout:      time.Duration(timeoutSeconds) * time.Second,
	}
}

// Stream handles POST /api/v1/chat/stream.
// It sets SSE headers, runs the orchestrator, and streams events to the client.
func (h *ChatHandler) Stream(c *gin.Context) {
	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed"})
		return
	}

	// Extract identity from JWT context (set by auth middleware).
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	// Set SSE headers — must happen BEFORE any write.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // bypass proxy buffering

	// Derive a context that is bounded by the per-request timeout.
	ctx := c.Request.Context()
	var cancel context.CancelFunc
	if h.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, h.timeout)
		defer cancel()
	}

	// Run the orchestrator with the timeout-bounded context.
	params := service.RunParams{
		TenantID:    tenantID,
		UserID:      userID,
		ThreadID:    req.ThreadID,
		Message:     req.Message,
		Attachments: req.Attachments,
		AgentID:     req.AgentID,
		Model:       req.Model,
	}

	eventCh := h.orchestrator.Run(ctx, params)

	// Stream events to client. Each event is marshaled as a JSON string
	// and emitted as an SSE "data:" line. Events are flushed immediately
	// so the client receives tokens as they arrive.
	c.Stream(func(w io.Writer) bool {
		event, ok := <-eventCh
		if !ok {
			return false // channel closed — stream complete
		}

		data, err := json.Marshal(event.Data)
		if err != nil {
			// Defensive: emit a structured error instead of silently
			// dropping the event or sending malformed JSON.
			data, _ = json.Marshal(map[string]string{
				"error": "internal: failed to marshal event data",
			})
		}

		c.SSEvent(event.Event, string(data))
		if flushErr := c.Writer.Flush(); flushErr != nil {
			// Client disconnected; the orchestrator will eventually
			// terminate via its context cancellation.
			return false
		}
		return true
	})
}
