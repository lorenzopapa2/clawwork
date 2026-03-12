package service

import (
	"log"

	"github.com/agenthub/server/model"
	"github.com/agenthub/server/store"
)

type PaymentService struct {
	store      *store.SQLiteStore
	blockchain *BlockchainService
}

func NewPaymentService(s *store.SQLiteStore, bc *BlockchainService) *PaymentService {
	return &PaymentService{store: s, blockchain: bc}
}

func (s *PaymentService) GetEscrowStatus(taskID string) (*model.EscrowInfo, error) {
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	// Try to read real on-chain status if blockchain is enabled
	if s.blockchain.IsEnabled() {
		onChain, err := s.blockchain.GetEscrowOnChain(task.ID)
		if err != nil {
			log.Printf("[payment] on-chain escrow read failed for %s: %v (falling back to DB)", taskID, err)
		} else {
			var status model.EscrowStatus
			switch onChain.Status {
			case 1:
				status = model.EscrowLocked
			case 2:
				status = model.EscrowReleased
			case 3:
				status = model.EscrowRefunded
			case 4:
				status = model.EscrowDisputed
			default:
				status = model.EscrowLocked
			}

			return &model.EscrowInfo{
				TaskID:       task.ID,
				EscrowAmount: task.Bounty,
				EscrowTx:     task.EscrowTx,
				Status:       status,
				CreatedAt:    task.CreatedAt,
			}, nil
		}
	}

	// Fallback: derive status from task status in DB
	var status model.EscrowStatus
	switch task.Status {
	case model.TaskCompleted:
		status = model.EscrowReleased
	case model.TaskDisputed:
		status = model.EscrowDisputed
	default:
		status = model.EscrowLocked
	}

	return &model.EscrowInfo{
		TaskID:       task.ID,
		EscrowAmount: task.Bounty,
		EscrowTx:     task.EscrowTx,
		Status:       status,
		CreatedAt:    task.CreatedAt,
	}, nil
}

func (s *PaymentService) ListHistory(agentID, payType string, page, limit int) ([]*model.Payment, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.store.ListPayments(agentID, payType, page, limit)
}
