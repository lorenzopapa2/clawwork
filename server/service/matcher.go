package service

import (
	"sort"

	"github.com/clawwork/server/model"
	"github.com/clawwork/server/store"
)

type MatcherService struct {
	store *store.SQLiteStore
}

func NewMatcherService(s *store.SQLiteStore) *MatcherService {
	return &MatcherService{store: s}
}

type MatchResult struct {
	Agent *model.Agent `json:"agent"`
	Score float64      `json:"score"`
}

// FindMatchingAgents returns agents sorted by match score for a given task.
func (s *MatcherService) FindMatchingAgents(task *model.Task, limit int) ([]MatchResult, error) {
	agents, _, err := s.store.ListAgents("online", "", 1, 100)
	if err != nil {
		return nil, err
	}

	var results []MatchResult

	for _, agent := range agents {
		// Skip the publisher
		if agent.ID == task.PublisherID {
			continue
		}

		score := s.calculateScore(agent, task)
		if score > 0 {
			results = append(results, MatchResult{
				Agent: agent,
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *MatcherService) calculateScore(agent *model.Agent, task *model.Task) float64 {
	score := 0.0

	// Capability match (0-50 points)
	capSet := make(map[string]bool)
	for _, c := range agent.Capabilities {
		capSet[c] = true
	}
	matched := 0
	for _, req := range task.Requirements {
		if capSet[req] {
			matched++
		}
	}
	if len(task.Requirements) > 0 {
		score += (float64(matched) / float64(len(task.Requirements))) * 50
	}

	// Reputation bonus (0-30 points)
	score += float64(agent.Reputation) * 0.3

	// Completion history bonus (0-20 points)
	total, completed, _ := s.store.CountCompletedByWorker(agent.ID)
	if total > 0 {
		score += (float64(completed) / float64(total)) * 20
	}

	return score
}
