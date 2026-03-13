package service

import (
	"fmt"
	"log"
	"math"
	"math/big"
	"time"

	"github.com/clawwork/server/model"
	"github.com/clawwork/server/store"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
)

type TaskService struct {
	store      *store.SQLiteStore
	blockchain *BlockchainService
}

func NewTaskService(s *store.SQLiteStore, bc *BlockchainService) *TaskService {
	return &TaskService{store: s, blockchain: bc}
}

func (s *TaskService) Create(publisherID string, req *model.CreateTaskReq) (*model.Task, error) {
	now := time.Now()
	taskID := req.ID
	if taskID == "" {
		taskID = "task_" + uuid.New().String()[:8]
	}

	task := &model.Task{
		ID:           taskID,
		PublisherID:  publisherID,
		Title:        req.Title,
		Description:  req.Description,
		Requirements: req.Requirements,
		Bounty:       req.Bounty,
		EscrowTx:     req.EscrowTx,
		MaxWorkers:   req.MaxWorkers,
		PaymentModel: req.PaymentModel,
		Status:       model.TaskOpen,
		Deadline:     req.Deadline,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Verify escrow transaction on-chain if blockchain is enabled
	if s.blockchain.IsEnabled() {
		event, err := s.blockchain.VerifyEscrowTx(req.EscrowTx)
		if err != nil {
			return nil, fmt.Errorf("escrow verification failed: %w", err)
		}
		log.Printf("[task] escrow verified on-chain: publisher=%s amount=%s", event.Publisher.Hex(), event.Amount.String())
	}

	if err := s.store.CreateTask(task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Record spending
	s.store.UpdateAgentSpent(publisherID, req.Bounty)

	return task, nil
}

func (s *TaskService) Get(id string) (*model.Task, error) {
	return s.store.GetTask(id)
}

func (s *TaskService) List(q *model.TaskListQuery) ([]*model.Task, int, error) {
	return s.store.ListTasks(q)
}

func (s *TaskService) GetBids(taskID string) ([]*model.Bid, error) {
	return s.store.GetBidsByTask(taskID)
}

func (s *TaskService) Bid(taskID, agentID string, req *model.BidReq) (*model.Bid, error) {
	// Check task exists and is open
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found")
	}
	if task.Status != model.TaskOpen {
		return nil, fmt.Errorf("task is not open for bidding")
	}

	// Check agent is not the publisher
	if task.PublisherID == agentID {
		return nil, fmt.Errorf("cannot bid on your own task")
	}

	// Check if already bid
	hasBid, _ := s.store.HasAgentBid(taskID, agentID)
	if hasBid {
		return nil, fmt.Errorf("already bid on this task")
	}

	bid := &model.Bid{
		ID:              "bid_" + uuid.New().String()[:8],
		TaskID:          taskID,
		AgentID:         agentID,
		Proposal:        req.Proposal,
		Price:           req.Price,
		EstimatedTokens: req.EstimatedTokens,
		Status:          model.BidPending,
		CreatedAt:       time.Now(),
	}

	if err := s.store.CreateBid(bid); err != nil {
		return nil, fmt.Errorf("failed to create bid: %w", err)
	}

	return bid, nil
}

func (s *TaskService) Assign(taskID, publisherID string, req *model.AssignReq) error {
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found")
	}
	if task.PublisherID != publisherID {
		return fmt.Errorf("not the task publisher")
	}
	if task.Status != model.TaskOpen {
		return fmt.Errorf("task is not open")
	}
	if len(req.BidIDs) > task.MaxWorkers {
		return fmt.Errorf("exceeds max workers limit")
	}

	// Accept selected bids, reject others
	allBids, _ := s.store.GetBidsByTask(taskID)
	selectedSet := make(map[string]bool)
	for _, id := range req.BidIDs {
		selectedSet[id] = true
	}

	for _, bid := range allBids {
		if selectedSet[bid.ID] {
			s.store.UpdateBidStatus(bid.ID, model.BidAccepted)
			s.store.AddTaskWorker(taskID, bid.AgentID)
		} else {
			s.store.UpdateBidStatus(bid.ID, model.BidRejected)
		}
	}

	s.store.UpdateTaskStatus(taskID, model.TaskAssigned)
	return nil
}

func (s *TaskService) Submit(taskID, agentID string, req *model.SubmitResultReq) error {
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found")
	}
	if task.Status != model.TaskAssigned && task.Status != model.TaskInProgress {
		return fmt.Errorf("task is not in assignable state")
	}

	// Verify agent is a worker
	isWorker, _ := s.store.IsTaskWorker(taskID, agentID)
	if !isWorker {
		return fmt.Errorf("not assigned to this task")
	}

	// Update result and token usage
	s.store.UpdateTaskResult(taskID, req.Result)
	s.store.UpdateWorkerTokens(taskID, agentID, req.TokenUsage.PromptTokens, req.TokenUsage.CompletionTokens)
	s.store.UpdateTaskStatus(taskID, model.TaskReview)

	return nil
}

func (s *TaskService) Approve(taskID, publisherID string, req *model.ApproveReq) ([]model.Distribution, error) {
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("task not found")
	}
	if task.PublisherID != publisherID {
		return nil, fmt.Errorf("not the task publisher")
	}
	if task.Status != model.TaskReview {
		return nil, fmt.Errorf("task is not in review")
	}

	workers, _ := s.store.GetTaskWorkers(taskID)
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers assigned")
	}

	// Calculate distributions
	platformFee := task.Bounty * 0.02
	distributable := task.Bounty - platformFee
	distributions := calculateDistributions(task, workers, distributable, req, s.store)

	// Execute on-chain release if blockchain is enabled
	if s.blockchain.IsEnabled() {
		workerAddrs, shares, err := s.buildOnChainParams(workers, distributions, task.Bounty)
		if err != nil {
			return nil, fmt.Errorf("failed to build on-chain params: %w", err)
		}

		releaseTxHash, err := s.blockchain.ReleasePayment(taskID, workerAddrs, shares)
		if err != nil {
			return nil, fmt.Errorf("on-chain release failed: %w", err)
		}

		// Update all distributions with the real tx hash
		for i := range distributions {
			distributions[i].TxHash = releaseTxHash
		}
		log.Printf("[task] on-chain release: taskID=%s txHash=%s", taskID, releaseTxHash)
	}

	// Record payments and update earnings
	for _, d := range distributions {
		payment := &model.Payment{
			ID:          "pay_" + uuid.New().String()[:8],
			TaskID:      taskID,
			FromAgent:   publisherID,
			ToAgent:     d.AgentID,
			Amount:      d.Amount,
			PlatformFee: platformFee / float64(len(distributions)),
			TxHash:      d.TxHash,
			Type:        model.PaymentTypeTaskPayment,
			CreatedAt:   time.Now(),
		}
		s.store.CreatePayment(payment)
		s.store.UpdateAgentEarned(d.AgentID, d.Amount)
		s.store.UpdateAgentReputation(d.AgentID, 2) // reward reputation
	}

	s.store.UpdateTaskStatus(taskID, model.TaskCompleted)
	s.store.UpdateAgentReputation(publisherID, 1) // publisher gets small rep boost

	return distributions, nil
}

func (s *TaskService) Dispute(taskID, agentID string, req *model.DisputeReq) error {
	task, err := s.store.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found")
	}

	// Either publisher or worker can dispute
	isWorker, _ := s.store.IsTaskWorker(taskID, agentID)
	if task.PublisherID != agentID && !isWorker {
		return fmt.Errorf("not authorized to dispute")
	}

	if task.Status != model.TaskReview && task.Status != model.TaskInProgress && task.Status != model.TaskAssigned {
		return fmt.Errorf("cannot dispute task in current state")
	}

	// Execute on-chain dispute if blockchain is enabled
	if s.blockchain.IsEnabled() {
		txHash, err := s.blockchain.DisputeOnChain(taskID)
		if err != nil {
			return fmt.Errorf("on-chain dispute failed: %w", err)
		}
		log.Printf("[task] on-chain dispute: taskID=%s txHash=%s", taskID, txHash)
	}

	s.store.UpdateTaskStatus(taskID, model.TaskDisputed)
	return nil
}

// buildOnChainParams converts distribution data into on-chain call parameters.
// The contract expects shares in USDT token decimals (18 decimals on BSC).
func (s *TaskService) buildOnChainParams(workerIDs []string, distributions []model.Distribution, bounty float64) ([]common.Address, []*big.Int, error) {
	// Build a map from agentID -> distribution amount
	distMap := make(map[string]float64)
	for _, d := range distributions {
		distMap[d.AgentID] = d.Amount
	}

	var addrs []common.Address
	var shares []*big.Int

	for _, wid := range workerIDs {
		agent, err := s.store.GetAgent(wid)
		if err != nil {
			return nil, nil, fmt.Errorf("worker %s not found: %w", wid, err)
		}
		if agent.WalletAddress == "" {
			return nil, nil, fmt.Errorf("worker %s has no wallet address", wid)
		}

		addrs = append(addrs, common.HexToAddress(agent.WalletAddress))

		amount := distMap[wid]
		// Convert float64 USDT amount to big.Int with 18 decimals
		shares = append(shares, toTokenAmount(amount))
	}

	return addrs, shares, nil
}

// toTokenAmount converts a float64 USDT amount to *big.Int with 18 decimals.
func toTokenAmount(amount float64) *big.Int {
	// amount * 1e18
	whole := math.Floor(amount)
	frac := amount - whole

	result := new(big.Int)
	wholeBig := new(big.Int).SetInt64(int64(whole))
	decimals := new(big.Int).SetInt64(1e18)
	result.Mul(wholeBig, decimals)

	// Add fractional part
	fracBig := new(big.Int).SetInt64(int64(math.Round(frac * 1e18)))
	result.Add(result, fracBig)

	return result
}

func calculateDistributions(task *model.Task, workers []string, distributable float64, req *model.ApproveReq, st *store.SQLiteStore) []model.Distribution {
	var distributions []model.Distribution

	switch task.PaymentModel {
	case model.PaymentTokenBased:
		usage, _ := st.GetWorkerTokenUsage(task.ID)
		totalTokens := 0
		for _, u := range usage {
			totalTokens += u.PromptTokens + u.CompletionTokens
		}
		if totalTokens == 0 {
			// fallback to equal split
			share := distributable / float64(len(workers))
			for _, w := range workers {
				distributions = append(distributions, model.Distribution{
					AgentID: w,
					Amount:  share,
					TxHash:  "pending_" + uuid.New().String()[:8],
				})
			}
		} else {
			for _, w := range workers {
				u := usage[w]
				workerTokens := u.PromptTokens + u.CompletionTokens
				share := (float64(workerTokens) / float64(totalTokens)) * distributable
				distributions = append(distributions, model.Distribution{
					AgentID: w,
					Amount:  share,
					TxHash:  "pending_" + uuid.New().String()[:8],
				})
			}
		}

	case model.PaymentWeighted:
		if req != nil && len(req.WorkerWeights) > 0 {
			totalWeight := 0
			for _, w := range req.WorkerWeights {
				totalWeight += w
			}
			for agentID, weight := range req.WorkerWeights {
				share := (float64(weight) / float64(totalWeight)) * distributable
				distributions = append(distributions, model.Distribution{
					AgentID: agentID,
					Amount:  share,
					TxHash:  "pending_" + uuid.New().String()[:8],
				})
			}
		} else {
			// fallback to equal
			share := distributable / float64(len(workers))
			for _, w := range workers {
				distributions = append(distributions, model.Distribution{
					AgentID: w,
					Amount:  share,
					TxHash:  "pending_" + uuid.New().String()[:8],
				})
			}
		}

	default: // fixed
		// For fixed, use accepted bid prices
		bids, _ := st.GetBidsByTask(task.ID)
		bidMap := make(map[string]*model.Bid)
		for _, b := range bids {
			if b.Status == model.BidAccepted {
				bidMap[b.AgentID] = b
			}
		}
		for _, w := range workers {
			amount := distributable / float64(len(workers))
			if bid, ok := bidMap[w]; ok {
				fee := bid.Price * 0.02
				amount = bid.Price - fee
			}
			distributions = append(distributions, model.Distribution{
				AgentID: w,
				Amount:  amount,
				TxHash:  "pending_" + uuid.New().String()[:8],
			})
		}
	}

	return distributions
}
