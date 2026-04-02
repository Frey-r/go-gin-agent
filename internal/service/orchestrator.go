package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/llm"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

const maxReasoningIterations = 10

// Orchestrator manages the reasoning loop: context rehidration, LLM calls,
// tool dispatch, and response streaming.
type Orchestrator struct {
	fabric      *llm.Fabric
	toolReg     *tools.Registry
	convStore   *store.ConversationStore
	promptStore *store.PromptStore
	telemetry   *TelemetryService
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	fabric *llm.Fabric,
	toolReg *tools.Registry,
	convStore *store.ConversationStore,
	promptStore *store.PromptStore,
	telemetry *TelemetryService,
) *Orchestrator {
	return &Orchestrator{
		fabric:      fabric,
		toolReg:     toolReg,
		convStore:   convStore,
		promptStore: promptStore,
		telemetry:   telemetry,
	}
}

// RunParams holds the parameters for a single orchestration run.
type RunParams struct {
	TenantID    string
	UserID      string
	ThreadID    string
	Message     string
	Attachments []model.Attachment
	Model       string // which LLM model to use
	PromptID    string // dynamic prompt to use (resolved from store)
	AgentID     string // agent to run (resolved from store)
}

// Run executes the full reasoning loop and returns a channel of SSE events.
// The channel is closed when processing is complete.
func (o *Orchestrator) Run(ctx context.Context, params RunParams) <-chan model.SSEvent {
	eventCh := make(chan model.SSEvent, 64)

	go func() {
		defer close(eventCh)
		o.executeLoop(ctx, params, eventCh)
	}()

	return eventCh
}

func (o *Orchestrator) executeLoop(ctx context.Context, params RunParams, eventCh chan<- model.SSEvent) {
	// 1. Get the LLM provider via fabric
	provider, err := o.fabric.GetProvider(params.Model)
	if err != nil {
		eventCh <- model.SSEvent{Event: "error", Data: "model not available"}
		return
	}

	// 2. Rehidrate conversation history
	history, err := o.convStore.GetHistory(ctx, params.ThreadID, 20)
	if err != nil {
		log.Error().Err(err).Str("thread_id", params.ThreadID).Msg("failed to get history")
		eventCh <- model.SSEvent{Event: "error", Data: "internal error"}
		return
	}

	// 3. Build messages array
	messages := o.buildMessages(history, params)

	// 4. Get tool definitions
	toolDefs := o.getToolDefs()

	// 5. Reasoning loop
	var fullResponse string
	var totalInputTokens, totalOutputTokens int

	for i := 0; i < maxReasoningIterations; i++ {
		select {
		case <-ctx.Done():
			eventCh <- model.SSEvent{Event: "error", Data: "request timeout"}
			return
		default:
		}

		streamCh, err := provider.ChatStream(ctx, messages, toolDefs)
		if err != nil {
			log.Error().Err(err).Msg("LLM stream error")
			eventCh <- model.SSEvent{Event: "error", Data: "inference error"}
			return
		}

		var hasToolCalls bool
		var iterationToolCalls []llm.ToolCall
		var iterationContent string

		for event := range streamCh {
			switch event.Type {
			case llm.EventToken:
				iterationContent += event.Content
				eventCh <- model.SSEvent{Event: "token", Data: event.Content}

			case llm.EventToolCall:
				hasToolCalls = true
				iterationToolCalls = append(iterationToolCalls, event.ToolCalls...)
				eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Ejecutando herramienta: %s", event.ToolCalls[0].Name)}

			case llm.EventDone:
				totalInputTokens += event.InputTokens
				totalOutputTokens += event.OutputTokens

			case llm.EventError:
				eventCh <- model.SSEvent{Event: "error", Data: event.Error}
				return
			}
		}

		if hasToolCalls {
			// Append assistant message with tool calls
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: iterationContent,
			})

			// Execute each tool and append results
			for _, tc := range iterationToolCalls {
				result, err := o.toolReg.Execute(ctx, tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error executing tool %s: %s", tc.Name, err.Error())
					log.Error().Err(err).Str("tool", tc.Name).Msg("tool execution failed")
				}
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}

			// Continue loop — the LLM needs to process tool results
			continue
		}

		// No tool calls — we have the final response
		fullResponse = iterationContent
		break
	}

	// 6. Persist messages
	o.persistExchange(ctx, params, fullResponse)

	// 7. Send done event
	eventCh <- model.SSEvent{Event: "done", Data: map[string]interface{}{
		"input_tokens":  totalInputTokens,
		"output_tokens": totalOutputTokens,
		"model":         params.Model,
	}}

	// 8. Async telemetry
	if o.telemetry != nil {
		o.telemetry.ReportAsync(TraceData{
			TenantID:     params.TenantID,
			UserID:       params.UserID,
			ThreadID:     params.ThreadID,
			Model:        params.Model,
			InputTokens:  totalInputTokens,
			OutputTokens: totalOutputTokens,
		})
	}
}

func (o *Orchestrator) buildMessages(history []model.ConversationMessage, params RunParams) []llm.Message {
	messages := make([]llm.Message, 0, len(history)+3)

	// System prompt — resolve dynamically from store, fallback to default
	systemPrompt := "Eres un asistente de IA especializado. Responde de forma precisa y útil."
	if params.PromptID != "" && o.promptStore != nil {
		org := params.TenantID
		if p, err := o.promptStore.GetActivePrompt(context.Background(), params.PromptID, org); err == nil && p != nil {
			systemPrompt = p.Content
		}
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: systemPrompt,
	})

	// History
	for _, msg := range history {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Ephemeral attachments (injected as user context)
	if len(params.Attachments) > 0 {
		var attachContent string
		for _, att := range params.Attachments {
			attachContent += fmt.Sprintf("\n--- Archivo: %s ---\n%s\n", att.Filename, att.Content)
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: "[Documentos adjuntos para contexto]" + attachContent,
		})
	}

	// User message
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: params.Message,
	})

	return messages
}

func (o *Orchestrator) getToolDefs() []llm.ToolDef {
	names := o.toolReg.List()
	if len(names) == 0 {
		return nil
	}

	// For now, return basic tool definitions
	// TODO: tools should self-describe their schemas
	defs := make([]llm.ToolDef, 0, len(names))
	for _, name := range names {
		defs = append(defs, llm.ToolDef{
			Name:        name,
			Description: fmt.Sprintf("Execute the %s tool", name),
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		})
	}
	return defs
}

func (o *Orchestrator) persistExchange(ctx context.Context, params RunParams, response string) {
	// Save user message
	userMsg := &model.ConversationMessage{
		ThreadID: params.ThreadID,
		TenantID: params.TenantID,
		UserID:   params.UserID,
		Role:     "user",
		Content:  params.Message,
	}
	if err := o.convStore.SaveMessage(ctx, userMsg); err != nil {
		log.Error().Err(err).Msg("failed to persist user message")
	}

	// Save assistant response
	if response != "" {
		assistantMsg := &model.ConversationMessage{
			ThreadID: params.ThreadID,
			TenantID: params.TenantID,
			UserID:   params.UserID,
			Role:     "assistant",
			Content:  response,
		}
		if err := o.convStore.SaveMessage(ctx, assistantMsg); err != nil {
			log.Error().Err(err).Msg("failed to persist assistant message")
		}
	}
}

// getToolDefsJSON is a helper for serializing tool definitions if needed.
func getToolDefsJSON(defs []llm.ToolDef) string {
	b, _ := json.Marshal(defs)
	return string(b)
}
