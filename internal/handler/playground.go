package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ebachmann/go-gin-agent/internal/agent"
	"github.com/ebachmann/go-gin-agent/internal/middleware"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store/playground"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

type PlaygroundHandler struct {
	agentStore    *playground.AgentStore
	convStore     *playground.ConversationStore
	runner        *agent.PlaygroundRunner
	httpExecutor  *tools.HTTPExecutor
}

func NewPlaygroundHandler(
	agentStore *playground.AgentStore,
	convStore *playground.ConversationStore,
	runner *agent.PlaygroundRunner,
	httpExecutor *tools.HTTPExecutor,
) *PlaygroundHandler {
	return &PlaygroundHandler{
		agentStore:   agentStore,
		convStore:    convStore,
		runner:       runner,
		httpExecutor: httpExecutor,
	}
}

func (h *PlaygroundHandler) CreateAgent(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)

	var req model.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agent, err := h.agentStore.Create(&req, visitorID)
	if err != nil {
		if err == playground.ErrMaxAgentsReached {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "maximum agents reached"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create agent"})
		return
	}

	c.JSON(http.StatusCreated, agent)
}

func (h *PlaygroundHandler) GetAgent(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)
	agentID := c.Param("agent_id")

	agent, err := h.agentStore.Get(agentID, visitorID)
	if err != nil {
		if err == playground.ErrAgentNotFound || err == playground.ErrAccessDenied {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agent"})
		return
	}

	c.JSON(http.StatusOK, agent)
}

func (h *PlaygroundHandler) DeleteAgent(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)
	agentID := c.Param("agent_id")

	err := h.agentStore.Delete(agentID, visitorID)
	if err != nil {
		if err == playground.ErrAgentNotFound || err == playground.ErrAccessDenied {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete agent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *PlaygroundHandler) GetConversation(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)
	convID := c.Param("conversation_id")

	conv, err := h.convStore.Get(convID, visitorID)
	if err != nil {
		if err == playground.ErrConversationNotFound || err == playground.ErrAccessDenied {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get conversation"})
		return
	}

	c.JSON(http.StatusOK, conv)
}

func (h *PlaygroundHandler) StreamChat(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)

	var req model.PlaygroundChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.agentStore.Get(req.AgentID, visitorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found or access denied"})
		return
	}

	var convID string
	if req.ConversationID != "" {
		conv, err := h.convStore.Get(req.ConversationID, visitorID)
		if err == nil && conv != nil && conv.AgentID == req.AgentID {
			convID = req.ConversationID
		}
	}

	if convID == "" {
		conv, err := h.convStore.GetOrCreate(req.AgentID, visitorID)
		if err != nil {
			if err == playground.ErrMaxConversationsReached {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": "maximum conversations reached"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create conversation"})
			return
		}
		convID = conv.ID
	}

	c.Stream(func(w io.Writer) bool {
		params := agent.PlaygroundRunParams{
			AgentID:        req.AgentID,
			VisitorID:      visitorID,
			ConversationID: convID,
			Message:        req.Message,
		}

		eventCh := h.runner.Run(c.Request.Context(), params)

		for event := range eventCh {
			var line string
			switch e := event.Data.(type) {
			case string:
				line = fmt.Sprintf("event: %s\ndata: %s\n\n", event.Event, e)
			default:
				line = fmt.Sprintf("event: %s\ndata: %v\n\n", event.Event, e)
			}
			w.Write([]byte(line))
		}

		return false
	})
}

func (h *PlaygroundHandler) Delegate(c *gin.Context) {
	visitorID := c.GetString(middleware.VisitorIDKey)
	sourceAgentID := c.Param("agent_id")

	var req model.DelegateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.agentStore.Get(sourceAgentID, visitorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source agent not found"})
		return
	}

	targetAgent, err := h.agentStore.Get(req.TargetAgentID, visitorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target agent not found"})
		return
	}

	params := agent.PlaygroundRunParams{
		AgentID:        req.TargetAgentID,
		VisitorID:      visitorID,
		ConversationID: "",
		Message:        req.Task,
		ParentAgentID:  sourceAgentID,
	}

	var response string
	eventCh := h.runner.Run(c.Request.Context(), params)
	for event := range eventCh {
		if event.Event == "token" {
			if content, ok := event.Data.(string); ok {
				response += content
			}
		}
		if event.Event == "error" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": event.Data})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"target_agent": targetAgent.Name,
		"response":     response,
	})
}