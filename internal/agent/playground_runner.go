package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/llm"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store/playground"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

type PlaygroundRunner struct {
	mu          sync.RWMutex
	fabric      *llm.Fabric
	agentStore  *playground.AgentStore
	convStore   *playground.ConversationStore
	httpExec    *tools.HTTPExecutor
	rateLimiter *visitorHTTPRateLimiter
}

type visitorHTTPRateLimiter struct {
	mu       sync.RWMutex
	lastCall map[string]time.Time
	window   time.Duration
}

func newVisitorHTTPRateLimiter(windowSecs int) *visitorHTTPRateLimiter {
	return &visitorHTTPRateLimiter{
		lastCall: make(map[string]time.Time),
		window:   time.Duration(windowSecs) * time.Second,
	}
}

func (rl *visitorHTTPRateLimiter) allow(visitorID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	last, exists := rl.lastCall[visitorID]
	now := time.Now()

	if exists && now.Sub(last) < rl.window {
		return false
	}

	rl.lastCall[visitorID] = now
	return true
}

func NewPlaygroundRunner(fabric *llm.Fabric, agentStore *playground.AgentStore, convStore *playground.ConversationStore, httpExec *tools.HTTPExecutor) *PlaygroundRunner {
	return &PlaygroundRunner{
		fabric:      fabric,
		agentStore:  agentStore,
		convStore:   convStore,
		httpExec:    httpExec,
		rateLimiter: newVisitorHTTPRateLimiter(10),
	}
}

type PlaygroundRunParams struct {
	AgentID        string
	VisitorID      string
	ConversationID string
	Message        string
	ParentAgentID  string
}

func (r *PlaygroundRunner) Run(ctx context.Context, params PlaygroundRunParams) <-chan model.SSEvent {
	eventCh := make(chan model.SSEvent, 64)

	go func() {
		defer close(eventCh)
		r.executeAgent(ctx, params, eventCh)
	}()

	return eventCh
}

func (r *PlaygroundRunner) executeAgent(ctx context.Context, params PlaygroundRunParams, eventCh chan<- model.SSEvent) {
	agent, err := r.agentStore.Get(params.AgentID, params.VisitorID)
	if err != nil {
		eventCh <- model.SSEvent{Event: "error", Data: fmt.Sprintf("agent not found or access denied")}
		return
	}

	provider, err := r.fabric.GetProvider(agent.Model)
	if err != nil {
		eventCh <- model.SSEvent{Event: "error", Data: "model not available"}
		return
	}

	var history []model.PGMessage
	if params.ConversationID != "" {
		conv, err := r.convStore.Get(params.ConversationID, params.VisitorID)
		if err == nil && conv != nil {
			history = conv.Messages
		}
	}

	messages := r.buildMessages(agent, history, params.Message)
	toolDefs := r.buildToolDefs(agent)

	for i := 0; i < 10; i++ {
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
			}
		}

		if hasToolCalls {
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: iterationContent})

			for _, tc := range iterationToolCalls {
				result := r.dispatchToolCall(ctx, agent, params.VisitorID, tc, eventCh)
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		if params.ConversationID != "" {
			r.convStore.AddMessage(params.ConversationID, model.PGMessage{
				Role:     "user",
				Content:  params.Message,
				Timestamp: time.Now(),
			})
			r.convStore.AddMessage(params.ConversationID, model.PGMessage{
				Role:     "assistant",
				Content:  iterationContent,
				Timestamp: time.Now(),
			})
		}

		eventCh <- model.SSEvent{Event: "done", Data: iterationContent}
		return
	}

	eventCh <- model.SSEvent{Event: "error", Data: "max iterations reached"}
}

func (r *PlaygroundRunner) dispatchToolCall(ctx context.Context, agent *model.PlaygroundAgent, visitorID string, tc llm.ToolCall, eventCh chan<- model.SSEvent) string {
	for _, tool := range agent.Tools {
		if tool.Name == tc.Name {
			if tool.Type == "http" {
				cfgJSON, _ := json.Marshal(tool.Config)
				return r.executeHTTPTool(ctx, visitorID, string(cfgJSON))
			}
		}
	}

	if len(agent.Tools) == 0 || tc.Name == "delegate" {
		for _, tool := range agent.Tools {
			if tool.Name == tc.Name && tool.Type == "delegate" {
				var args map[string]string
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				targetID := args["agent_id"]
				task := args["task"]

				eventCh <- model.SSEvent{Event: "status", Data: fmt.Sprintf("Delegando a: %s", targetID)}

				subParams := PlaygroundRunParams{
					AgentID:       targetID,
					VisitorID:     visitorID,
					Message:       task,
					ParentAgentID: agent.ID,
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
				return result
			}
		}
	}

	return fmt.Sprintf("tool %q not found", tc.Name)
}

func (r *PlaygroundRunner) executeHTTPTool(ctx context.Context, visitorID string, configJSON string) string {
	if !r.rateLimiter.allow(visitorID) {
		return "HTTP tool rate limited: max 1 call per 10 seconds per visitor"
	}

	if r.httpExec == nil {
		return "HTTP tool not configured"
	}

	result, err := r.httpExec.Execute(ctx, configJSON)
	if err != nil {
		log.Error().Err(err).Msg("playground HTTP tool failed")
		return fmt.Sprintf("HTTP error: %s", err.Error())
	}

	return result
}

func (r *PlaygroundRunner) buildMessages(agent *model.PlaygroundAgent, history []model.PGMessage, userMsg string) []llm.Message {
	messages := make([]llm.Message, 0, len(history)+3)

	if agent.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: agent.SystemPrompt,
		})
	}

	for _, msg := range history {
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userMsg,
	})

	return messages
}

func (r *PlaygroundRunner) buildToolDefs(agent *model.PlaygroundAgent) []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(agent.Tools)+1)

	for _, tool := range agent.Tools {
		def := llm.ToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		}

		if tool.Type == "http" {
			def.Parameters = map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url":     map[string]interface{}{"type": "string", "description": "URL to call"},
					"method":  map[string]interface{}{"type": "string", "description": "HTTP method"},
					"headers": map[string]interface{}{"type": "object", "description": "HTTP headers"},
					"body":    map[string]interface{}{"type": "object", "description": "Request body"},
				},
			}
		}

		defs = append(defs, def)
	}

	return defs
}