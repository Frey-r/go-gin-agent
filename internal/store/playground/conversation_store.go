package playground

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

type ConversationStore struct {
	mu            sync.RWMutex
	conversations map[string]*model.PlaygroundConversation
	byVisitor     map[string][]string
	cfg           ConversationStoreConfig
	stopCh        chan struct{}
}

type ConversationStoreConfig struct {
	TTLMinutes     int
	MaxConvPerIP   int
	CleanupSecs    int
}

func NewConversationStore(cfg ConversationStoreConfig) *ConversationStore {
	store := &ConversationStore{
		conversations: make(map[string]*model.PlaygroundConversation),
		byVisitor:     make(map[string][]string),
		cfg:           cfg,
		stopCh:        make(chan struct{}),
	}

	go store.cleanupLoop()
	return store
}

func (s *ConversationStore) GetOrCreate(agentID, visitorID string) (*model.PlaygroundConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	owned := s.byVisitor[visitorID]
	if len(owned) >= s.cfg.MaxConvPerIP {
		return nil, ErrMaxConversationsReached
	}

	convID := uuid.NewString()
	now := time.Now()
	conv := &model.PlaygroundConversation{
		ID:        convID,
		AgentID:   agentID,
		VisitorID: visitorID,
		Messages:  make([]model.PGMessage, 0),
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(s.cfg.TTLMinutes) * time.Minute),
	}

	s.conversations[convID] = conv
	s.byVisitor[visitorID] = append(owned, convID)

	return conv, nil
}

func (s *ConversationStore) Get(id string, visitorID string) (*model.PlaygroundConversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[id]
	if !ok {
		return nil, ErrConversationNotFound
	}

	if conv.VisitorID != visitorID {
		return nil, ErrAccessDenied
	}

	conv.ExpiresAt = time.Now().Add(time.Duration(s.cfg.TTLMinutes) * time.Minute)

	return conv, nil
}

func (s *ConversationStore) AddMessage(convID string, msg model.PGMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return ErrConversationNotFound
	}

	conv.Messages = append(conv.Messages, msg)
	conv.ExpiresAt = time.Now().Add(time.Duration(s.cfg.TTLMinutes) * time.Minute)

	return nil
}

func (s *ConversationStore) Delete(id string, visitorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[id]
	if !ok {
		return ErrConversationNotFound
	}

	if conv.VisitorID != visitorID {
		return ErrAccessDenied
	}

	delete(s.conversations, id)
	owned := s.byVisitor[visitorID]
	for i, cid := range owned {
		if cid == id {
			s.byVisitor[visitorID] = append(owned[:i], owned[i+1:]...)
			break
		}
	}

	return nil
}

func (s *ConversationStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, conv := range s.conversations {
		if now.After(conv.ExpiresAt) {
			delete(s.conversations, id)
			owned := s.byVisitor[conv.VisitorID]
			for i, cid := range owned {
				if cid == id {
					s.byVisitor[conv.VisitorID] = append(owned[:i], owned[i+1:]...)
					break
				}
			}
			log.Info().Str("conversation_id", id).Msg("playground: conversation expired and cleaned up")
		}
	}
}

func (s *ConversationStore) cleanupLoop() {
	ticker := time.NewTicker(time.Duration(s.cfg.CleanupSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *ConversationStore) Stop() {
	close(s.stopCh)
}