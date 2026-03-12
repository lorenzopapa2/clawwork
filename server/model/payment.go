package model

import "time"

type PaymentType string

const (
	PaymentTypeTaskPayment PaymentType = "task_payment"
	PaymentTypeRefund      PaymentType = "refund"
	PaymentTypeFee         PaymentType = "platform_fee"
)

type EscrowStatus string

const (
	EscrowLocked   EscrowStatus = "locked"
	EscrowReleased EscrowStatus = "released"
	EscrowRefunded EscrowStatus = "refunded"
	EscrowDisputed EscrowStatus = "disputed"
)

type Payment struct {
	ID          string      `json:"id"`
	TaskID      string      `json:"task_id"`
	FromAgent   string      `json:"from_agent"`
	ToAgent     string      `json:"to_agent"`
	Amount      float64     `json:"amount"`
	PlatformFee float64     `json:"platform_fee"`
	TxHash      string      `json:"tx_hash"`
	Type        PaymentType `json:"type"`
	CreatedAt   time.Time   `json:"created_at"`
}

type EscrowInfo struct {
	TaskID       string       `json:"task_id"`
	EscrowAmount float64      `json:"escrow_amount"`
	EscrowTx     string       `json:"escrow_tx"`
	Status       EscrowStatus `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`
}

type Distribution struct {
	AgentID string  `json:"agent_id"`
	Amount  float64 `json:"amount"`
	TxHash  string  `json:"tx_hash"`
}

// --- Response types ---

type PaginatedResponse struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

type ErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Details string `json:"details,omitempty"`
	} `json:"error"`
}
