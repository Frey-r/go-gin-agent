package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

// PromptStore handles prompt and agent definition persistence.
type PromptStore struct {
	db *DB
}

// NewPromptStore creates a new PromptStore.
func NewPromptStore(db *DB) *PromptStore {
	return &PromptStore{db: db}
}

// ────────────────────────────────────────────────────────────
// Prompt CRUD
// ────────────────────────────────────────────────────────────

// UpsertPrompt creates a new prompt version or updates the active one.
// It deactivates any previous active version for the same prompt_id+org,
// then inserts the new one as active.
func (s *PromptStore) UpsertPrompt(ctx context.Context, prompt *model.Prompt) error {
	tx, err := s.db.Conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get current max version
	var maxVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM prompts WHERE prompt_id = ? AND organization = ?`,
		prompt.PromptID, prompt.Organization,
	).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("get max version: %w", err)
	}

	// Deactivate previous active version
	_, err = tx.ExecContext(ctx,
		`UPDATE prompts SET is_active = 0, updated_at = CURRENT_TIMESTAMP
		 WHERE prompt_id = ? AND organization = ? AND is_active = 1`,
		prompt.PromptID, prompt.Organization,
	)
	if err != nil {
		return fmt.Errorf("deactivate previous: %w", err)
	}

	// Insert new version
	prompt.Version = maxVersion + 1
	prompt.IsActive = true

	_, err = tx.ExecContext(ctx,
		`INSERT INTO prompts (id, prompt_id, organization, content, version, is_active, metadata, created_by)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		prompt.ID, prompt.PromptID, prompt.Organization, prompt.Content,
		prompt.Version, prompt.Metadata, prompt.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert prompt: %w", err)
	}

	return tx.Commit()
}

// GetActivePrompt retrieves the active prompt for a given prompt_id and organization.
func (s *PromptStore) GetActivePrompt(ctx context.Context, promptID, organization string) (*model.Prompt, error) {
	query := `SELECT id, prompt_id, organization, content, version, is_active, metadata, created_by, created_at, updated_at
			  FROM prompts
			  WHERE prompt_id = ? AND organization = ? AND is_active = 1`

	var p model.Prompt
	err := s.db.Conn.QueryRowContext(ctx, query, promptID, organization).Scan(
		&p.ID, &p.PromptID, &p.Organization, &p.Content,
		&p.Version, &p.IsActive, &p.Metadata, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active prompt: %w", err)
	}
	return &p, nil
}

// ListPrompts returns all active prompts for an organization.
func (s *PromptStore) ListPrompts(ctx context.Context, organization string) ([]model.Prompt, error) {
	query := `SELECT id, prompt_id, organization, content, version, is_active, metadata, created_by, created_at, updated_at
			  FROM prompts
			  WHERE organization = ? AND is_active = 1
			  ORDER BY prompt_id`

	rows, err := s.db.Conn.QueryContext(ctx, query, organization)
	if err != nil {
		return nil, fmt.Errorf("list prompts: %w", err)
	}
	defer rows.Close()

	var prompts []model.Prompt
	for rows.Next() {
		var p model.Prompt
		if err := rows.Scan(
			&p.ID, &p.PromptID, &p.Organization, &p.Content,
			&p.Version, &p.IsActive, &p.Metadata, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan prompt: %w", err)
		}
		prompts = append(prompts, p)
	}
	return prompts, rows.Err()
}

// GetPromptHistory returns all versions of a prompt (active and inactive).
func (s *PromptStore) GetPromptHistory(ctx context.Context, promptID, organization string) ([]model.Prompt, error) {
	query := `SELECT id, prompt_id, organization, content, version, is_active, metadata, created_by, created_at, updated_at
			  FROM prompts
			  WHERE prompt_id = ? AND organization = ?
			  ORDER BY version DESC`

	rows, err := s.db.Conn.QueryContext(ctx, query, promptID, organization)
	if err != nil {
		return nil, fmt.Errorf("get prompt history: %w", err)
	}
	defer rows.Close()

	var prompts []model.Prompt
	for rows.Next() {
		var p model.Prompt
		if err := rows.Scan(
			&p.ID, &p.PromptID, &p.Organization, &p.Content,
			&p.Version, &p.IsActive, &p.Metadata, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan prompt: %w", err)
		}
		prompts = append(prompts, p)
	}
	return prompts, rows.Err()
}

// DeletePrompt soft-deletes a prompt by deactivating all its versions.
func (s *PromptStore) DeletePrompt(ctx context.Context, promptID, organization string) error {
	_, err := s.db.Conn.ExecContext(ctx,
		`UPDATE prompts SET is_active = 0, updated_at = CURRENT_TIMESTAMP
		 WHERE prompt_id = ? AND organization = ?`,
		promptID, organization,
	)
	if err != nil {
		return fmt.Errorf("delete prompt: %w", err)
	}
	return nil
}

// ────────────────────────────────────────────────────────────
// Agent Definition CRUD
// ────────────────────────────────────────────────────────────

// UpsertAgent creates or updates an agent definition.
func (s *PromptStore) UpsertAgent(ctx context.Context, agent *model.AgentDef) error {
	tx, err := s.db.Conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Deactivate previous version
	_, err = tx.ExecContext(ctx,
		`UPDATE agents SET is_active = 0, updated_at = CURRENT_TIMESTAMP
		 WHERE agent_id = ? AND organization = ? AND is_active = 1`,
		agent.AgentID, agent.Organization,
	)
	if err != nil {
		return fmt.Errorf("deactivate previous agent: %w", err)
	}

	// Insert new version
	_, err = tx.ExecContext(ctx,
		`INSERT INTO agents (id, agent_id, organization, name, description, prompt_id, model, max_iterations, tools, sub_agents, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		agent.ID, agent.AgentID, agent.Organization, agent.Name, agent.Description,
		agent.PromptID, agent.Model, agent.MaxIterations, agent.Tools, agent.SubAgents,
	)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}

	return tx.Commit()
}

// GetActiveAgent retrieves the active agent definition.
func (s *PromptStore) GetActiveAgent(ctx context.Context, agentID, organization string) (*model.AgentDef, error) {
	query := `SELECT id, agent_id, organization, name, description, prompt_id, model, max_iterations, tools, sub_agents, is_active, created_at, updated_at
			  FROM agents
			  WHERE agent_id = ? AND organization = ? AND is_active = 1`

	var a model.AgentDef
	err := s.db.Conn.QueryRowContext(ctx, query, agentID, organization).Scan(
		&a.ID, &a.AgentID, &a.Organization, &a.Name, &a.Description,
		&a.PromptID, &a.Model, &a.MaxIterations, &a.Tools, &a.SubAgents,
		&a.IsActive, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active agent: %w", err)
	}
	return &a, nil
}

// ListAgents returns all active agents for an organization.
func (s *PromptStore) ListAgents(ctx context.Context, organization string) ([]model.AgentDef, error) {
	query := `SELECT id, agent_id, organization, name, description, prompt_id, model, max_iterations, tools, sub_agents, is_active, created_at, updated_at
			  FROM agents
			  WHERE organization = ? AND is_active = 1
			  ORDER BY agent_id`

	rows, err := s.db.Conn.QueryContext(ctx, query, organization)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []model.AgentDef
	for rows.Next() {
		var a model.AgentDef
		if err := rows.Scan(
			&a.ID, &a.AgentID, &a.Organization, &a.Name, &a.Description,
			&a.PromptID, &a.Model, &a.MaxIterations, &a.Tools, &a.SubAgents,
			&a.IsActive, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}
