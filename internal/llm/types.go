package llm

// Role constants for LLM messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message represents a single message in the conversation sent to the LLM.
type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"` // for tool responses
}

// ToolDef defines a tool/function the LLM can request to call.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// ToolCall represents the LLM's request to invoke a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// StreamEventType enumerates the types of events emitted during streaming.
type StreamEventType string

const (
	EventToken    StreamEventType = "token"
	EventToolCall StreamEventType = "tool_call"
	EventDone     StreamEventType = "done"
	EventError    StreamEventType = "error"
)

// StreamEvent is a single event in the streaming response from the LLM.
type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	Content   string          `json:"content,omitempty"`    // for token events
	ToolCalls []ToolCall      `json:"tool_calls,omitempty"` // for tool_call events
	Error     string          `json:"error,omitempty"`      // for error events

	// Usage stats (populated on EventDone)
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Model        string `json:"model,omitempty"`
}
