package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/llm"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

const defaultMaxIterations = 10

// resolvedConfig holds agent-derived data that must be computed before
// the reasoning-loop goroutine starts, so that getToolDefs and
// buildMessages operate with the correct per-agent context.
type resolvedConfig struct {
	Agent        *model.AgentDef // nil if no agent was resolved
	ToolNames    []string        // tool names from AgentDef.Tools (JSON); empty means "all registered"
	SystemPrompt string          // resolved system prompt
	Model        string          // resolved model name
}

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
	Model       string // which LLM model to use; resolved from agent if empty
	PromptID    string // dynamic prompt to use; resolved from agent if empty
	AgentID     string // agent to run; triggers full config resolution from DB
}

// Run executes the full reasoning loop and returns a channel of SSE events.
// The channel is closed when processing is complete.
func (o *Orchestrator) Run(ctx context.Context, params RunParams) <-chan model.SSEvent {
	eventCh := make(chan model.SSEvent, 64)

	// Resolve agent config BEFORE the goroutine so that getToolDefs
	// has the correct per-agent tool list and the system prompt.
	cfg := o.resolveConfig(ctx, params)

	// If model is still empty at this point, let the fabric apply its fallback.
	if cfg.Model == "" {
		cfg.Model = params.Model
	}

	go func() {
		defer close(eventCh)
		o.executeLoop(ctx, params, cfg, eventCh)
	}()

	return eventCh
}

// resolveConfig loads the agent definition, resolves the system prompt,
// and parses the agent's tool list. All of this happens synchronously
// in Run before the goroutine is spawned.
func (o *Orchestrator) resolveConfig(ctx context.Context, params RunParams) resolvedConfig {
	cfg := resolvedConfig{Model: params.Model}

	if params.AgentID == "" || o.promptStore == nil {
		// No agent requested — resolve prompt only if PromptID is set.
		cfg.SystemPrompt = params.PromptID
		if cfg.SystemPrompt != "" {
			cfg.SystemPrompt = o.resolveSystemPrompt(ctx, cfg.SystemPrompt, params.TenantID)
		}
		if cfg.SystemPrompt == "" {
			cfg.SystemPrompt = "Eres un asistente de IA especializado. Responde de forma precisa y útil."
		}
		return cfg
	}

	agent, err := o.promptStore.GetActiveAgent(ctx, params.AgentID, params.TenantID)
	if err != nil {
		log.Error().Err(err).Str("agent_id", params.AgentID).Msg("failed to load agent definition")
		// Fall through with empty agent; use whatever params were given directly.
		cfg.SystemPrompt = o.resolveSystemPrompt(ctx, params.PromptID, params.TenantID)
		if cfg.SystemPrompt == "" {
			cfg.SystemPrompt = "Eres un asistente de IA especializado. Responde de forma precisa y útil."
		}
		return cfg
	}

	cfg.Agent = agent

	// Model: explicit param overrides agent default.
	if cfg.Model == "" {
		cfg.Model = agent.Model
	}

	// System prompt: resolve from agent's PromptID.
	cfg.SystemPrompt = o.resolveSystemPrompt(ctx, agent.PromptID, params.TenantID)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Eres un asistente de IA especializado. Responde de forma precisa y útil."
	}

	// Parse the Tools JSON array from the agent definition.
	if agent.Tools != nil && *agent.Tools != "" {
		if err := json.Unmarshal([]byte(*agent.Tools), &cfg.ToolNames); err != nil {
			log.Warn().Err(err).Str("agent_id", params.AgentID).Msg("failed to parse agent tools JSON; defaulting to all registered tools")
			cfg.ToolNames = nil // nil means "all registered tools"
		}
	}

	log.Info().
		Str("agent_id", params.AgentID).
		Str("model", cfg.Model).
		Int("tool_count", len(cfg.ToolNames)).
		Msg("resolved agent configuration")

	return cfg
}

// resolveSystemPrompt fetches the prompt content for the given promptID and org.
// Returns the empty string if the prompt cannot be resolved.
func (o *Orchestrator) resolveSystemPrompt(ctx context.Context, promptID, org string) string {
	if promptID == "" || o.promptStore == nil {
		return ""
	}
	p, err := o.promptStore.GetActivePrompt(ctx, promptID, org)
	if err != nil || p == nil {
		return ""
	}
	return p.Content
}

func (o *Orchestrator) executeLoop(ctx context.Context, params RunParams, cfg resolvedConfig, eventCh chan<- model.SSEvent) {
	// 1. Get LLM provider.
	provider, err := o.fabric.GetProvider(cfg.Model)
	if err != nil {
		eventCh <- model.SSEvent{Event: "error", Data: fmt.Sprintf("model not available: %s", cfg.Model)}
		return
	}

	// 2. Rehidrate conversation history.
	history, err := o.convStore.GetHistory(ctx, params.ThreadID, 20)
	if err != nil {
		log.Error().Err(err).Str("thread_id", params.ThreadID).Msg("failed to get history")
		eventCh <- model.SSEvent{Event: "error", Data: "internal error"}
		return
	}

	// 3. Build messages with the pre-resolved system prompt.
	messages := o.buildMessages(ctx, history, params, cfg.SystemPrompt)

	// 4. Get tool definitions filtered to the agent's allowed tools.
	toolDefs := o.getToolDefs(cfg.ToolNames)
	if len(toolDefs) > 0 {
		eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Herramientas activas: %d", len(toolDefs))}
	}

	// 5. Determine iteration limit.
	maxIterations := defaultMaxIterations
	if cfg.Agent != nil && cfg.Agent.MaxIterations > 0 {
		maxIterations = cfg.Agent.MaxIterations
	}

	// 6. Reasoning loop.
	var fullResponse string
	var totalInputTokens, totalOutputTokens int

	for i := 0; i < maxIterations; i++ {
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
				for _, tc := range event.ToolCalls {
					eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Ejecutando herramienta: %s", tc.Name)}
				}

			case llm.EventDone:
				totalInputTokens += event.InputTokens
				totalOutputTokens += event.OutputTokens

			case llm.EventError:
				eventCh <- model.SSEvent{Event: "error", Data: event.Error}
				return
			}
		}

		if hasToolCalls {
			// Append assistant message with tool calls.
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: iterationContent,
			})

			// Execute each tool and append results.
			for _, tc := range iterationToolCalls {
				result, err := o.toolReg.Execute(ctx, tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf(`{"error": "tool %q execution failed: %s"}`, tc.Name, err.Error())
					log.Error().Err(err).Str("tool", tc.Name).Int("iteration", i).Msg("tool execution failed")
				}
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}

			// Continue loop — the LLM processes tool results.
			continue
		}

		// No tool calls — final response.
		fullResponse = iterationContent
		break
	}

	if fullResponse == "" && maxIterations > 0 {
		log.Warn().Int("iterations", maxIterations).Msg("reasoning loop exited with no response")
	}

	// 7. Persist messages.
	o.persistExchange(ctx, params, fullResponse)

	// 8. Send done event.
	eventCh <- model.SSEvent{Event: "done", Data: map[string]interface{}{
		"input_tokens":   totalInputTokens,
		"output_tokens": totalOutputTokens,
		"model":          cfg.Model,
		"agent_id":       params.AgentID,
	}}

	// 9. Async telemetry.
	if o.telemetry != nil {
		o.telemetry.ReportAsync(TraceData{
			TenantID:     params.TenantID,
			UserID:       params.UserID,
			ThreadID:     params.ThreadID,
			Model:        cfg.Model,
			InputTokens:  totalInputTokens,
			OutputTokens: totalOutputTokens,
		})
	}
}

// buildMessages assembles the message list for the LLM.
// It accepts the already-resolved system prompt so that it does not
// need to derive a context for store lookups.
func (o *Orchestrator) buildMessages(ctx context.Context, history []model.ConversationMessage, params RunParams, systemPrompt string) []llm.Message {
	messages := make([]llm.Message, 0, len(history)+3)

	// System prompt — already resolved by resolveConfig.
	if systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: systemPrompt,
		})
	}

	// Conversation history.
	for _, msg := range history {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Ephemeral attachments injected as user context.
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

	// User message.
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: params.Message,
	})

	return messages
}

// getToolDefs returns tool definitions filtered to the names in toolNames.
// If toolNames is nil or empty, all registered tools are returned.
func (o *Orchestrator) getToolDefs(toolNames []string) []llm.ToolDef {
	registered := o.toolReg.List()
	if len(registered) == 0 {
		return nil
	}

	// If the agent specified an explicit list, intersect with registered tools.
	// This ensures we never surface a tool the registry doesn't know about.
	var names []string
	if len(toolNames) > 0 {
		// Sort both slices for deterministic O(n+m) intersection.
		slices.Sort(registered)
		slices.Sort(toolNames)
		names = intersectSorted(registered, toolNames)
		if len(names) == 0 {
			log.Debug().Msg("agent tool list does not match any registered tools")
			return nil
		}
	} else {
		names = registered
	}

	// TODO: tools should self-describe their parameter schemas instead of
	// returning an empty Parameters object. When that is implemented,
	// replace this generic loop with a per-tool lookup.
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

// intersectSorted returns the set intersection of two sorted string slices.
func intersectSorted(a, b []string) []string {
	var result []string
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			i++
		} else if a[i] > b[j] {
			j++
		} else {
			result = append(result, a[i])
			i++
			j++
		}
	}
	return result
}

func (o *Orchestrator) persistExchange(ctx context.Context, params RunParams, response string) {
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
