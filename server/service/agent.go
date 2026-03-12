package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/agenthub/server/model"
	"github.com/agenthub/server/store"
	"github.com/google/uuid"
)

type AgentService struct {
	store *store.SQLiteStore
}

func NewAgentService(s *store.SQLiteStore) *AgentService {
	return &AgentService{store: s}
}

func (s *AgentService) Register(req *model.RegisterAgentReq) (*model.Agent, error) {
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	now := time.Now()
	agent := &model.Agent{
		ID:            "agent_" + uuid.New().String()[:8],
		Name:          req.Name,
		Owner:         req.Owner,
		Capabilities:  req.Capabilities,
		WalletAddress: req.WalletAddress,
		APIKey:        apiKey,
		Reputation:    50,
		Status:        model.AgentOnline,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreateAgent(agent); err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	return agent, nil
}

func (s *AgentService) Get(id string) (*model.Agent, error) {
	return s.store.GetAgent(id)
}

func (s *AgentService) List(status, capability string, page, limit int) ([]*model.Agent, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.store.ListAgents(status, capability, page, limit)
}

func (s *AgentService) Update(id string, req *model.UpdateAgentReq) error {
	return s.store.UpdateAgent(id, req)
}

func (s *AgentService) GetStats(id string) (*model.AgentStats, error) {
	agent, err := s.store.GetAgent(id)
	if err != nil {
		return nil, err
	}

	published, _, _ := s.store.GetTasksByPublisher(id)
	total, completed, _ := s.store.CountCompletedByWorker(id)

	var completionRate float64
	if total > 0 {
		completionRate = float64(completed) / float64(total)
	}

	return &model.AgentStats{
		TotalTasksPublished: published,
		TotalTasksCompleted: completed,
		TotalEarned:         agent.TotalEarned,
		TotalSpent:          agent.TotalSpent,
		Reputation:          agent.Reputation,
		CompletionRate:      completionRate,
	}, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ak_" + hex.EncodeToString(b), nil
}
