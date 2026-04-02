package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store"
)

// PromptHandler handles prompt management endpoints.
type PromptHandler struct {
	promptStore *store.PromptStore
	validate    *validator.Validate
}

// NewPromptHandler creates a new PromptHandler.
func NewPromptHandler(promptStore *store.PromptStore) *PromptHandler {
	return &PromptHandler{
		promptStore: promptStore,
		validate:    validator.New(),
	}
}

// ────────────────────────────────────────────────────────────
// Prompt Endpoints
// ────────────────────────────────────────────────────────────

// UpsertPrompt handles PUT /api/v1/prompts.
// Creates a new version of a prompt or creates the first version.
// The previous active version is automatically deactivated.
func (h *PromptHandler) UpsertPrompt(c *gin.Context) {
	var req model.UpsertPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "details": formatValidationErrors(err)})
		return
	}

	userID := c.GetString("user_id")

	prompt := &model.Prompt{
		ID:           uuid.New().String(),
		PromptID:     req.PromptID,
		Organization: req.Organization,
		Content:      req.Content,
		Metadata:     req.Metadata,
		CreatedBy:    &userID,
	}

	if err := h.promptStore.UpsertPrompt(c.Request.Context(), prompt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save prompt"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "prompt updated",
		"prompt":  prompt,
	})
}

// GetPrompt handles GET /api/v1/prompts/:prompt_id.
// Returns the active version of a prompt for the caller's organization.
func (h *PromptHandler) GetPrompt(c *gin.Context) {
	promptID := c.Param("prompt_id")
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	prompt, err := h.promptStore.GetActivePrompt(c.Request.Context(), promptID, org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get prompt"})
		return
	}
	if prompt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "prompt not found"})
		return
	}

	c.JSON(http.StatusOK, prompt)
}

// ListPrompts handles GET /api/v1/prompts.
// Returns all active prompts for the given organization.
func (h *PromptHandler) ListPrompts(c *gin.Context) {
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	prompts, err := h.promptStore.ListPrompts(c.Request.Context(), org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list prompts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"prompts": prompts,
		"count":   len(prompts),
	})
}

// GetPromptHistory handles GET /api/v1/prompts/:prompt_id/history.
// Returns all versions (active and inactive) of a prompt.
func (h *PromptHandler) GetPromptHistory(c *gin.Context) {
	promptID := c.Param("prompt_id")
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	history, err := h.promptStore.GetPromptHistory(c.Request.Context(), promptID, org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"versions": history,
		"count":    len(history),
	})
}

// DeletePrompt handles DELETE /api/v1/prompts/:prompt_id.
// Soft-deletes by deactivating all versions.
func (h *PromptHandler) DeletePrompt(c *gin.Context) {
	promptID := c.Param("prompt_id")
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	if err := h.promptStore.DeletePrompt(c.Request.Context(), promptID, org); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete prompt"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "prompt deactivated"})
}

// ────────────────────────────────────────────────────────────
// Agent Definition Endpoints
// ────────────────────────────────────────────────────────────

// UpsertAgent handles PUT /api/v1/agents.
func (h *PromptHandler) UpsertAgent(c *gin.Context) {
	var req model.UpsertAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "details": formatValidationErrors(err)})
		return
	}

	maxIter := req.MaxIterations
	if maxIter == 0 {
		maxIter = 10
	}

	agent := &model.AgentDef{
		ID:            uuid.New().String(),
		AgentID:       req.AgentID,
		Organization:  req.Organization,
		Name:          req.Name,
		Description:   req.Description,
		PromptID:      req.PromptID,
		Model:         req.Model,
		MaxIterations: maxIter,
		Tools:         req.Tools,
		SubAgents:     req.SubAgents,
	}

	if err := h.promptStore.UpsertAgent(c.Request.Context(), agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save agent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "agent updated",
		"agent":   agent,
	})
}

// GetAgent handles GET /api/v1/agents/:agent_id.
func (h *PromptHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	agent, err := h.promptStore.GetActiveAgent(c.Request.Context(), agentID, org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get agent"})
		return
	}
	if agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	c.JSON(http.StatusOK, agent)
}

// ListAgents handles GET /api/v1/agents.
func (h *PromptHandler) ListAgents(c *gin.Context) {
	org := c.Query("organization")
	if org == "" {
		org = c.GetString("tenant_id")
	}

	agents, err := h.promptStore.ListAgents(c.Request.Context(), org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"count":  len(agents),
	})
}
