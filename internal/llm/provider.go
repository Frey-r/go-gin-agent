package llm

import "context"

// Provider is the interface all LLM backends must implement.
// The orchestrator depends on this abstraction, never on a concrete vendor.
type Provider interface {
	// ChatStream sends messages to the LLM and returns a channel of streaming events.
	// The channel is closed when the response is complete.
	// Callers must respect context cancellation.
	ChatStream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)

	// Name returns the provider identifier (e.g. "gemini", "grok").
	Name() string
}
