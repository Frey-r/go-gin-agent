package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// WebhookHandler handles incoming webhook integrations.
type WebhookHandler struct {
	secret string // HMAC secret for signature validation
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(secret string) *WebhookHandler {
	return &WebhookHandler{secret: secret}
}

// Handle processes POST /api/v1/webhooks.
// Validates the HMAC signature of the request body before processing.
func (h *WebhookHandler) Handle(c *gin.Context) {
	// Read body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unable to read body"})
		return
	}

	// Validate signature if secret is configured
	if h.secret != "" {
		signature := c.GetHeader("X-Webhook-Signature")
		if !h.validateSignature(body, signature) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	log.Info().
		Str("request_id", c.GetString("request_id")).
		Int("body_size", len(body)).
		Msg("webhook received")

	// TODO: Process webhook payload based on event type
	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// validateSignature checks the HMAC-SHA256 signature of the body.
func (h *WebhookHandler) validateSignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}
