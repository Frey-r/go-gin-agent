package playground

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

type AgentStore struct {
	mu       sync.RWMutex
	agents   map[string]*model.PlaygroundAgent
	byOwner  map[string][]string
	cfg      AgentStoreConfig
	stopCh   chan struct{}
}

type AgentStoreConfig struct {
	TTLMinutes     int
	MaxAgentsPerIP int
	CleanupSecs    int
}

func NewAgentStore(cfg AgentStoreConfig) *AgentStore {
	store := &AgentStore{
		agents:  make(map[string]*model.PlaygroundAgent),
		byOwner: make(map[string][]string),
		cfg:     cfg,
		stopCh:  make(chan struct{}),
	}

	go store.cleanupLoop()
	return store
}

func (s *AgentStore) Create(req *model.CreateAgentRequest, visitorID string) (*model.PlaygroundAgent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	owned := s.byOwner[visitorID]
	if len(owned) >= s.cfg.MaxAgentsPerIP {
		return nil, ErrMaxAgentsReached
	}

	now := time.Now()
	agent := &model.PlaygroundAgent{
		ID:            uuid.NewString(),
		Name:          req.Name,
		Description:   req.Description,
		SystemPrompt:  req.SystemPrompt,
		Model:         req.Model,
		Tools:         req.Tools,
		CreatedBy:     visitorID,
		CreatedAt:     now,
		LastAccessedAt: now,
		ExpiresAt:     now.Add(time.Duration(s.cfg.TTLMinutes) * time.Minute),
	}

	s.agents[agent.ID] = agent
	s.byOwner[visitorID] = append(owned, agent.ID)

	return agent, nil
}

func (s *AgentStore) Get(id string, visitorID string) (*model.PlaygroundAgent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[id]
	if !ok {
		return nil, ErrAgentNotFound
	}

	if agent.CreatedBy != visitorID {
		return nil, ErrAccessDenied
	}

	agent.LastAccessedAt = time.Now()
	agent.ExpiresAt = time.Now().Add(time.Duration(s.cfg.TTLMinutes) * time.Minute)

	return agent, nil
}

func (s *AgentStore) Delete(id string, visitorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[id]
	if !ok {
		return ErrAgentNotFound
	}

	if agent.CreatedBy != visitorID {
		return ErrAccessDenied
	}

	delete(s.agents, id)
	owned := s.byOwner[visitorID]
	for i, aid := range owned {
		if aid == id {
			s.byOwner[visitorID] = append(owned[:i], owned[i+1:]...)
			break
		}
	}

	return nil
}

func (s *AgentStore) CountByVisitor(visitorID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byOwner[visitorID])
}

func (s *AgentStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, agent := range s.agents {
		if now.After(agent.ExpiresAt) {
			delete(s.agents, id)
			owned := s.byOwner[agent.CreatedBy]
			for i, aid := range owned {
				if aid == id {
					s.byOwner[agent.CreatedBy] = append(owned[:i], owned[i+1:]...)
					break
				}
			}
			log.Info().Str("agent_id", id).Msg("playground: agent expired and cleaned up")
		}
	}
}

func (s *AgentStore) cleanupLoop() {
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

func (s *AgentStore) Stop() {
	close(s.stopCh)
}