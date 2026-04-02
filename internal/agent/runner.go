package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/llm"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

// Agent is a runtime instance of an agent definition.
// It combines a system prompt, an LLM provider, a set of tools, and
// optionally a set of sub-agents it can delegate to.
type Agent struct {
	ID            string
	Name          string
	Description   string
	SystemPrompt  string
	Model         string
	MaxIterations int
	ToolNames     []string   // tools this agent can use
	SubAgentIDs   []string   // sub-agents this agent can invoke
}

// Runner executes agents with their specific configuration.
// It resolves prompts from the store, builds the tool set per-agent,
// and handles sub-agent delegation as tool calls.
type Runner struct {
	mu          sync.RWMutex
	fabric      *llm.Fabric
	toolReg     *tools.Registry
	promptStore *store.PromptStore
	convStore   *store.ConversationStore

	// Sub-agent registry: agent_id -> Agent definition
	// Populated at startup and refreshed on demand
	agents map[string]*Agent
}

// NewRunner creates a new agent runner.
func NewRunner(
	fabric *llm.Fabric,
	toolReg *tools.Registry,
	promptStore *store.PromptStore,
	convStore *store.ConversationStore,
) *Runner {
	return &Runner{
		fabric:      fabric,
		toolReg:     toolReg,
		promptStore: promptStore,
		convStore:   convStore,
		agents:      make(map[string]*Agent),
	}
}

// LoadAgents loads all active agent definitions for an organization from the DB,
// resolves their prompts, and registers sub-agents as callable tools.
func (r *Runner) LoadAgents(ctx context.Context, organization string) error {
	agentDefs, err := r.promptStore.ListAgents(ctx, organization)
	if err != nil {
		return fmt.Errorf("load agents: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, def := range agentDefs {
		// Resolve the prompt
		prompt, err := r.promptStore.GetActivePrompt(ctx, def.PromptID, organization)
		if err != nil {
			log.Error().Err(err).Str("agent", def.AgentID).Str("prompt", def.PromptID).Msg("failed to resolve prompt")
			continue
		}

		systemPrompt := ""
		if prompt != nil {
			systemPrompt = prompt.Content
		}

		// Parse tool names JSON
		var toolNames []string
		if def.Tools != nil {
			_ = json.Unmarshal([]byte(*def.Tools), &toolNames)
		}

		// Parse sub-agent IDs JSON
		var subAgentIDs []string
		if def.SubAgents != nil {
			_ = json.Unmarshal([]byte(*def.SubAgents), &subAgentIDs)
		}

		agent := &Agent{
			ID:            def.AgentID,
			Name:          def.Name,
			Description:   def.Description,
			SystemPrompt:  systemPrompt,
			Model:         def.Model,
			MaxIterations: def.MaxIterations,
			ToolNames:     toolNames,
			SubAgentIDs:   subAgentIDs,
		}

		r.agents[def.AgentID] = agent
		log.Info().Str("agent", def.AgentID).Int("tools", len(toolNames)).Int("sub_agents", len(subAgentIDs)).Msg("loaded agent")
	}

	return nil
}

// GetAgent returns a loaded agent by ID.
func (r *Runner) GetAgent(agentID string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[agentID]
	return a, ok
}

// RunParams holds parameters for a single agent execution.
type RunParams struct {
	AgentID      string
	TenantID     string
	UserID       string
	ThreadID     string
	Message      string
	Attachments  []model.Attachment
	ParentTaskID string // set when called as sub-agent
}

// Run executes an agent's reasoning loop and returns SSE events.
func (r *Runner) Run(ctx context.Context, params RunParams) <-chan model.SSEvent {
	eventCh := make(chan model.SSEvent, 64)

	go func() {
		defer close(eventCh)
		r.executeAgent(ctx, params, eventCh)
	}()

	return eventCh
}

func (r *Runner) executeAgent(ctx context.Context, params RunParams, eventCh chan<- model.SSEvent) {
	// 1. Resolve agent
	agent, ok := r.GetAgent(params.AgentID)
	if !ok {
		// Fallback: try to load a prompt directly as a simple agent
		prompt, err := r.promptStore.GetActivePrompt(ctx, params.AgentID, params.TenantID)
		if err != nil || prompt == nil {
			eventCh <- model.SSEvent{Event: "error", Data: fmt.Sprintf("agent %q not found", params.AgentID)}
			return
		}
		// Create ephemeral agent from prompt
		agent = &Agent{
			ID:            params.AgentID,
			Name:          params.AgentID,
			SystemPrompt:  prompt.Content,
			Model:         "gemini-2.5-pro",
			MaxIterations: 10,
		}
	}

	// 2. Get LLM provider
	provider, err := r.fabric.GetProvider(agent.Model)
	if err != nil {
		eventCh <- model.SSEvent{Event: "error", Data: "model not available"}
		return
	}

	// 3. Get conversation history
	history, err := r.convStore.GetHistory(ctx, params.ThreadID, 20)
	if err != nil {
		log.Error().Err(err).Msg("failed to get history")
		eventCh <- model.SSEvent{Event: "error", Data: "internal error"}
		return
	}

	// 4. Build messages with the agent's system prompt
	messages := r.buildMessages(agent, history, params)

	// 5. Build tool definitions (filtered to this agent's allowed tools + sub-agents as tools)
	toolDefs := r.buildAgentTools(agent)

	// 6. Reasoning loop
	var fullResponse string

	for i := 0; i < agent.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			eventCh <- model.SSEvent{Event: "error", Data: "timeout"}
			return
		default:
		}

		streamCh, err := provider.ChatStream(ctx, messages, toolDefs)
		if err != nil {
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
				eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Ejecutando: %s", event.ToolCalls[0].Name)}
			case llm.EventError:
				eventCh <- model.SSEvent{Event: "error", Data: event.Error}
				return
			case llm.EventDone:
				// usage tracking handled upstream
			}
		}

		if hasToolCalls {
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: iterationContent})

			for _, tc := range iterationToolCalls {
				result := r.dispatchToolCall(ctx, agent, tc, params, eventCh)
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		fullResponse = iterationContent
		break
	}

	// 7. Persist (only if not a sub-agent call)
	if params.ParentTaskID == "" {
		r.persistExchange(ctx, params, fullResponse)
	}

	eventCh <- model.SSEvent{Event: "done", Data: nil}
}

// dispatchToolCall routes a tool call to either a real tool or a sub-agent.
func (r *Runner) dispatchToolCall(ctx context.Context, agent *Agent, tc llm.ToolCall, parentParams RunParams, eventCh chan<- model.SSEvent) string {
	// Check if this is a sub-agent call
	for _, subID := range agent.SubAgentIDs {
		if tc.Name == "delegate_to_"+subID || tc.Name == subID {
			// Extract the task from arguments
			var args map[string]string
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			task := args["task"]
			if task == "" {
				task = tc.Arguments
			}

			eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Delegando a sub-agente: %s", subID)}

			// Run sub-agent synchronously and collect its output
			subParams := RunParams{
				AgentID:      subID,
				TenantID:     parentParams.TenantID,
				UserID:       parentParams.UserID,
				ThreadID:     parentParams.ThreadID,
				Message:      task,
				ParentTaskID: agent.ID,
			}

			var result string
			subCh := r.Run(ctx, subParams)
			for subEvent := range subCh {
				if subEvent.Event == "token" {
					if content, ok := subEvent.Data.(string); ok {
						result += content
					}
				}
			}

			if result == "" {
				return fmt.Sprintf("Sub-agent %s returned no response", subID)
			}
			return result
		}
	}

	// Regular tool call
	result, err := r.toolReg.Execute(ctx, tc.Name, tc.Arguments)
	if err != nil {
		log.Error().Err(err).Str("tool", tc.Name).Msg("tool execution failed")
		return fmt.Sprintf("Error: %s", err.Error())
	}
	return result
}

func (r *Runner) buildMessages(agent *Agent, history []model.ConversationMessage, params RunParams) []llm.Message {
	messages := make([]llm.Message, 0, len(history)+3)

	// System prompt from the agent's configuration
	if agent.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: agent.SystemPrompt,
		})
	}

	// History
	for _, msg := range history {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Attachments
	if len(params.Attachments) > 0 {
		var attachContent string
		for _, att := range params.Attachments {
			attachContent += fmt.Sprintf("\n--- %s ---\n%s\n", att.Filename, att.Content)
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: "[Documentos adjuntos]" + attachContent,
		})
	}

	// User message
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: params.Message,
	})

	return messages
}

// buildAgentTools creates tool definitions filtered to this agent,
// plus synthetic tools for each sub-agent it can call.
func (r *Runner) buildAgentTools(agent *Agent) []llm.ToolDef {
	var defs []llm.ToolDef

	// Real tools (filtered to agent's allowed list)
	for _, name := range agent.ToolNames {
		defs = append(defs, llm.ToolDef{
			Name:        name,
			Description: fmt.Sprintf("Execute the %s tool", name),
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		})
	}

	// Sub-agents exposed as tools
	r.mu.RLock()
	for _, subID := range agent.SubAgentIDs {
		if sub, ok := r.agents[subID]; ok {
			defs = append(defs, llm.ToolDef{
				Name:        "delegate_to_" + subID,
				Description: fmt.Sprintf("Delegate a task to the %s agent. %s", sub.Name, sub.Description),
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"task": map[string]interface{}{
							"type":        "string",
							"description": "The task to delegate to this agent",
						},
					},
					"required": []string{"task"},
				},
			})
		}
	}
	r.mu.RUnlock()

	return defs
}

func (r *Runner) persistExchange(ctx context.Context, params RunParams, response string) {
	userMsg := &model.ConversationMessage{
		ThreadID: params.ThreadID,
		TenantID: params.TenantID,
		UserID:   params.UserID,
		Role:     "user",
		Content:  params.Message,
	}
	if err := r.convStore.SaveMessage(ctx, userMsg); err != nil {
		log.Error().Err(err).Msg("persist user message failed")
	}

	if response != "" {
		assistantMsg := &model.ConversationMessage{
			ThreadID: params.ThreadID,
			TenantID: params.TenantID,
			UserID:   params.UserID,
			Role:     "assistant",
			Content:  response,
		}
		if err := r.convStore.SaveMessage(ctx, assistantMsg); err != nil {
			log.Error().Err(err).Msg("persist assistant message failed")
		}
	}
}
