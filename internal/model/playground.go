package model

import (
	"time"
)

type PlaygroundAgent struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	SystemPrompt  string            `json:"system_prompt"`
	Model         string            `json:"model"`
	Tools         []ToolDefinition  `json:"tools"`
	CreatedBy     string            `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
	LastAccessedAt time.Time        `json:"last_accessed_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
}

type ToolDefinition struct {
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Config      HTTPToolConfig `json:"config"`
}

type HTTPToolConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Query   map[string]string `json:"query_params"`
	Body    interface{}       `json:"body,omitempty"`
}

type PlaygroundConversation struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	VisitorID string         `json:"visitor_id"`
	Messages  []PGMessage    `json:"messages"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
}

type PGMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type CreateAgentRequest struct {
	Name         string           `json:"name" validate:"required,min=1,max=100"`
	Description  string           `json:"description" validate:"max=500"`
	SystemPrompt string           `json:"system_prompt" validate:"required,min=1,max=50000"`
	Model        string           `json:"model" validate:"required"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

type PlaygroundChatRequest struct {
	AgentID        string `json:"agent_id" validate:"required"`
	ConversationID string `json:"conversation_id,omitempty"`
	Message        string `json:"message" validate:"required,min=1,max=10000"`
}

type DelegateRequest struct {
	TargetAgentID string `json:"target_agent_id" validate:"required"`
	Task          string `json:"task" validate:"required,min=1,max=50000"`
}

type ChatResponse struct {
	ConversationID string   `json:"conversation_id"`
	AgentID        string   `json:"agent_id"`
	TokensUsed     int      `json:"tokens_used,omitempty"`
	Cost           float64  `json:"cost,omitempty"`
}