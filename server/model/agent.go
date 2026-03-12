package model

import "time"

type AgentStatus string

const (
	AgentOnline  AgentStatus = "online"
	AgentOffline AgentStatus = "offline"
	AgentBusy    AgentStatus = "busy"
)

type Agent struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Owner         string      `json:"owner"`
	Capabilities  []string    `json:"capabilities"`
	WalletAddress string      `json:"wallet_address"`
	APIKey        string      `json:"api_key,omitempty"`
	Reputation    int         `json:"reputation"`
	TotalEarned   float64     `json:"total_earned"`
	TotalSpent    float64     `json:"total_spent"`
	Status        AgentStatus `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

type AgentStats struct {
	TotalTasksPublished int     `json:"total_tasks_published"`
	TotalTasksCompleted int     `json:"total_tasks_completed"`
	TotalEarned         float64 `json:"total_earned"`
	TotalSpent          float64 `json:"total_spent"`
	Reputation          int     `json:"reputation"`
	CompletionRate      float64 `json:"completion_rate"`
}

// --- Request/Response types ---

type RegisterAgentReq struct {
	Name          string   `json:"name" binding:"required"`
	Owner         string   `json:"owner" binding:"required"`
	Capabilities  []string `json:"capabilities" binding:"required"`
	WalletAddress string   `json:"wallet_address" binding:"required"`
}

type UpdateAgentReq struct {
	Name         string      `json:"name"`
	Capabilities []string    `json:"capabilities"`
	Status       AgentStatus `json:"status"`
}
