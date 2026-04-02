package model

import "time"

// ConversationMessage represents a single message in a conversation thread.
type ConversationMessage struct {
	ID         int64     `json:"id" db:"id"`
	ThreadID   string    `json:"thread_id" db:"thread_id"`
	TenantID   string    `json:"tenant_id" db:"tenant_id"`
	UserID     string    `json:"user_id" db:"user_id"`
	Role       string    `json:"role" db:"role"` // "user" | "assistant" | "system" | "tool"
	Content    string    `json:"content" db:"content"`
	ToolCallID *string   `json:"tool_call_id,omitempty" db:"tool_call_id"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// ChatRequest is the payload for POST /api/v1/chat/stream.
type ChatRequest struct {
	ThreadID string   `json:"thread_id" validate:"required,uuid"`
	Message  string   `json:"message" validate:"required,min=1,max=32000"`
	// Attachments are optional text files injected as context (ephemeral strategy).
	Attachments []Attachment `json:"attachments,omitempty" validate:"max=5,dive"`
}

// Attachment represents an ephemeral file uploaded as inline context.
type Attachment struct {
	Filename string `json:"filename" validate:"required,max=255"`
	Content  string `json:"content" validate:"required,max=100000"`
}

// SSEvent is a Server-Sent Event emitted during streaming.
type SSEvent struct {
	Event string      `json:"event"`           // "token", "status", "done", "error"
	Data  interface{} `json:"data"`
}
