package model

import "time"

type TaskStatus string

const (
	TaskOpen       TaskStatus = "open"
	TaskAssigned   TaskStatus = "assigned"
	TaskInProgress TaskStatus = "in_progress"
	TaskReview     TaskStatus = "review"
	TaskCompleted  TaskStatus = "completed"
	TaskDisputed   TaskStatus = "disputed"
)

type PaymentModel string

const (
	PaymentFixed      PaymentModel = "fixed"
	PaymentTokenBased PaymentModel = "token_based"
	PaymentWeighted   PaymentModel = "weighted"
)

type Task struct {
	ID           string       `json:"id"`
	PublisherID  string       `json:"publisher_id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Requirements []string     `json:"requirements"`
	Bounty       float64      `json:"bounty"`
	EscrowTx     string       `json:"escrow_tx"`
	MaxWorkers   int          `json:"max_workers"`
	PaymentModel PaymentModel `json:"payment_model"`
	Status       TaskStatus   `json:"status"`
	Deadline     time.Time    `json:"deadline"`
	Result       string       `json:"result,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type BidStatus string

const (
	BidPending  BidStatus = "pending"
	BidAccepted BidStatus = "accepted"
	BidRejected BidStatus = "rejected"
)

type Bid struct {
	ID              string    `json:"id"`
	TaskID          string    `json:"task_id"`
	AgentID         string    `json:"agent_id"`
	AgentName       string    `json:"agent_name,omitempty"`
	Proposal        string    `json:"proposal"`
	Price           float64   `json:"price"`
	EstimatedTokens int       `json:"estimated_tokens"`
	Status          BidStatus `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Request/Response types ---

type CreateTaskReq struct {
	ID           string       `json:"id,omitempty"`
	Title        string       `json:"title" binding:"required"`
	Description  string       `json:"description" binding:"required"`
	Requirements []string     `json:"requirements" binding:"required"`
	Bounty       float64      `json:"bounty" binding:"required,gt=0"`
	EscrowTx     string       `json:"escrow_tx" binding:"required"`
	MaxWorkers   int          `json:"max_workers" binding:"required,gte=1"`
	PaymentModel PaymentModel `json:"payment_model" binding:"required"`
	Deadline     time.Time    `json:"deadline" binding:"required"`
}

type BidReq struct {
	Proposal        string  `json:"proposal" binding:"required"`
	Price           float64 `json:"price" binding:"required,gt=0"`
	EstimatedTokens int     `json:"estimated_tokens"`
}

type AssignReq struct {
	BidIDs []string `json:"bid_ids" binding:"required"`
}

type SubmitResultReq struct {
	Result     string     `json:"result" binding:"required"`
	TokenUsage TokenUsage `json:"token_usage"`
}

type ApproveReq struct {
	WorkerWeights map[string]int `json:"worker_weights,omitempty"`
}

type DisputeReq struct {
	Reason string `json:"reason" binding:"required"`
}

type TaskListQuery struct {
	Status     string  `form:"status"`
	Capability string  `form:"capability"`
	MinBounty  float64 `form:"min_bounty"`
	MaxBounty  float64 `form:"max_bounty"`
	Publisher  string  `form:"publisher_id"`
	Sort       string  `form:"sort"`
	Page       int     `form:"page"`
	Limit      int     `form:"limit"`
}
