package store

import (
	"context"
	"fmt"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

// ConversationStore handles conversation message persistence.
type ConversationStore struct {
	db *DB
}

// NewConversationStore creates a new ConversationStore.
func NewConversationStore(db *DB) *ConversationStore {
	return &ConversationStore{db: db}
}

// SaveMessage persists a single conversation message.
func (s *ConversationStore) SaveMessage(ctx context.Context, msg *model.ConversationMessage) error {
	query := `INSERT INTO conversations (thread_id, tenant_id, user_id, role, content, tool_call_id)
			  VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.Conn.ExecContext(ctx, query,
		msg.ThreadID, msg.TenantID, msg.UserID, msg.Role, msg.Content, msg.ToolCallID)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

// GetHistory retrieves the last `limit` messages for a given thread, ordered chronologically.
func (s *ConversationStore) GetHistory(ctx context.Context, threadID string, limit int) ([]model.ConversationMessage, error) {
	query := `SELECT id, thread_id, tenant_id, user_id, role, content, tool_call_id, created_at
			  FROM conversations
			  WHERE thread_id = ?
			  ORDER BY created_at DESC
			  LIMIT ?`

	rows, err := s.db.Conn.QueryContext(ctx, query, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer rows.Close()

	var messages []model.ConversationMessage
	for rows.Next() {
		var msg model.ConversationMessage
		if err := rows.Scan(
			&msg.ID, &msg.ThreadID, &msg.TenantID, &msg.UserID,
			&msg.Role, &msg.Content, &msg.ToolCallID, &msg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	// Reverse to chronological order (we fetched DESC for LIMIT efficiency)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}
