package model

import "time"

// Prompt represents a managed system prompt that can be updated externally.
type Prompt struct {
	ID           string     `json:"id" db:"id"`
	PromptID     string     `json:"prompt_id" db:"prompt_id"`         // semantic slug: "sales-assistant"
	Organization string     `json:"organization" db:"organization"`   // tenant/org owner
	Content      string     `json:"content" db:"content"`             // the actual prompt text
	Version      int        `json:"version" db:"version"`
	IsActive     bool       `json:"is_active" db:"is_active"`
	Metadata     *string    `json:"metadata,omitempty" db:"metadata"` // free-form JSON
	CreatedBy    *string    `json:"created_by,omitempty" db:"created_by"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// UpsertPromptRequest is the payload for creating or updating a prompt.
type UpsertPromptRequest struct {
	PromptID     string  `json:"prompt_id" validate:"required,min=2,max=100"`
	Organization string  `json:"organization" validate:"required,min=2,max=100"`
	Content      string  `json:"content" validate:"required,min=1,max=100000"`
	Metadata     *string `json:"metadata,omitempty"`
}

// AgentDef represents a configured agent with its prompt, model, tools, and sub-agents.
type AgentDef struct {
	ID            string    `json:"id" db:"id"`
	AgentID       string    `json:"agent_id" db:"agent_id"`             // slug: "researcher"
	Organization  string    `json:"organization" db:"organization"`
	Name          string    `json:"name" db:"name"`
	Description   string    `json:"description,omitempty" db:"description"`
	PromptID      string    `json:"prompt_id" db:"prompt_id"`           // which prompt to use
	Model         string    `json:"model" db:"model"`                   // LLM model
	MaxIterations int       `json:"max_iterations" db:"max_iterations"`
	Tools         *string   `json:"tools,omitempty" db:"tools"`         // JSON array of tool names
	SubAgents     *string   `json:"sub_agents,omitempty" db:"sub_agents"` // JSON array of agent_ids
	IsActive      bool      `json:"is_active" db:"is_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// UpsertAgentRequest is the payload for creating or updating an agent definition.
type UpsertAgentRequest struct {
	AgentID       string  `json:"agent_id" validate:"required,min=2,max=100"`
	Organization  string  `json:"organization" validate:"required,min=2,max=100"`
	Name          string  `json:"name" validate:"required,min=2,max=200"`
	Description   string  `json:"description,omitempty"`
	PromptID      string  `json:"prompt_id" validate:"required"`
	Model         string  `json:"model" validate:"required"`
	MaxIterations int     `json:"max_iterations,omitempty"`
	Tools         *string `json:"tools,omitempty"`      // JSON string: ["search_knowledge", "run_script"]
	SubAgents     *string `json:"sub_agents,omitempty"` // JSON string: ["researcher", "coder"]
}
