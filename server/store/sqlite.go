package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/clawwork/server/model"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			owner TEXT NOT NULL,
			capabilities TEXT NOT NULL DEFAULT '[]',
			wallet_address TEXT NOT NULL,
			api_key TEXT UNIQUE NOT NULL,
			reputation INTEGER NOT NULL DEFAULT 50,
			total_earned REAL NOT NULL DEFAULT 0,
			total_spent REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'online',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_api_key ON agents(api_key)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status)`,

		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			publisher_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			requirements TEXT NOT NULL DEFAULT '[]',
			bounty REAL NOT NULL,
			escrow_tx TEXT NOT NULL,
			max_workers INTEGER NOT NULL DEFAULT 1,
			payment_model TEXT NOT NULL DEFAULT 'fixed',
			status TEXT NOT NULL DEFAULT 'open',
			deadline DATETIME NOT NULL,
			result TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (publisher_id) REFERENCES agents(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_publisher ON tasks(publisher_id)`,

		`CREATE TABLE IF NOT EXISTS bids (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			proposal TEXT NOT NULL,
			price REAL NOT NULL,
			estimated_tokens INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id),
			FOREIGN KEY (agent_id) REFERENCES agents(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bids_task ON bids(task_id)`,

		`CREATE TABLE IF NOT EXISTS task_workers (
			task_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (task_id, agent_id),
			FOREIGN KEY (task_id) REFERENCES tasks(id),
			FOREIGN KEY (agent_id) REFERENCES agents(id)
		)`,

		`CREATE TABLE IF NOT EXISTS payments (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			from_agent TEXT,
			to_agent TEXT,
			amount REAL NOT NULL,
			platform_fee REAL NOT NULL DEFAULT 0,
			tx_hash TEXT,
			type TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_task ON payments(task_id)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration error: %w\nquery: %s", err, q)
		}
	}
	return nil
}

// ==================== Agent ====================

func (s *SQLiteStore) CreateAgent(a *model.Agent) error {
	caps, _ := json.Marshal(a.Capabilities)
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, owner, capabilities, wallet_address, api_key, reputation, total_earned, total_spent, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Owner, string(caps), a.WalletAddress, a.APIKey,
		a.Reputation, a.TotalEarned, a.TotalSpent, a.Status,
		a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetAgent(id string) (*model.Agent, error) {
	row := s.db.QueryRow(`SELECT id, name, owner, capabilities, wallet_address, api_key, reputation, total_earned, total_spent, status, created_at, updated_at FROM agents WHERE id = ?`, id)
	return scanAgent(row)
}

func (s *SQLiteStore) GetAgentByAPIKey(apiKey string) (*model.Agent, error) {
	row := s.db.QueryRow(`SELECT id, name, owner, capabilities, wallet_address, api_key, reputation, total_earned, total_spent, status, created_at, updated_at FROM agents WHERE api_key = ?`, apiKey)
	return scanAgent(row)
}

func (s *SQLiteStore) ListAgents(status, capability string, page, limit int) ([]*model.Agent, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}

	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if capability != "" {
		where = append(where, "capabilities LIKE ?")
		args = append(args, "%\""+capability+"\"%")
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM agents WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(
		"SELECT id, name, owner, capabilities, wallet_address, api_key, reputation, total_earned, total_spent, status, created_at, updated_at FROM agents WHERE "+whereClause+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var agents []*model.Agent
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, 0, err
		}
		agents = append(agents, a)
	}
	return agents, total, nil
}

func (s *SQLiteStore) UpdateAgent(id string, req *model.UpdateAgentReq) error {
	sets := []string{"updated_at = ?"}
	args := []interface{}{time.Now()}

	if req.Name != "" {
		sets = append(sets, "name = ?")
		args = append(args, req.Name)
	}
	if req.Capabilities != nil {
		caps, _ := json.Marshal(req.Capabilities)
		sets = append(sets, "capabilities = ?")
		args = append(args, string(caps))
	}
	if req.Status != "" {
		sets = append(sets, "status = ?")
		args = append(args, req.Status)
	}

	args = append(args, id)
	_, err := s.db.Exec("UPDATE agents SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

func (s *SQLiteStore) UpdateAgentEarned(id string, amount float64) error {
	_, err := s.db.Exec("UPDATE agents SET total_earned = total_earned + ?, updated_at = ? WHERE id = ?", amount, time.Now(), id)
	return err
}

func (s *SQLiteStore) UpdateAgentSpent(id string, amount float64) error {
	_, err := s.db.Exec("UPDATE agents SET total_spent = total_spent + ?, updated_at = ? WHERE id = ?", amount, time.Now(), id)
	return err
}

func (s *SQLiteStore) UpdateAgentReputation(id string, delta int) error {
	_, err := s.db.Exec("UPDATE agents SET reputation = MIN(100, MAX(0, reputation + ?)), updated_at = ? WHERE id = ?", delta, time.Now(), id)
	return err
}

// ==================== Task ====================

func (s *SQLiteStore) CreateTask(t *model.Task) error {
	reqs, _ := json.Marshal(t.Requirements)
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, publisher_id, title, description, requirements, bounty, escrow_tx, max_workers, payment_model, status, deadline, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.PublisherID, t.Title, t.Description, string(reqs),
		t.Bounty, t.EscrowTx, t.MaxWorkers, t.PaymentModel,
		t.Status, t.Deadline, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetTask(id string) (*model.Task, error) {
	row := s.db.QueryRow(`SELECT id, publisher_id, title, description, requirements, bounty, escrow_tx, max_workers, payment_model, status, deadline, result, created_at, updated_at FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (s *SQLiteStore) ListTasks(q *model.TaskListQuery) ([]*model.Task, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}

	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.Capability != "" {
		where = append(where, "requirements LIKE ?")
		args = append(args, "%\""+q.Capability+"\"%")
	}
	if q.MinBounty > 0 {
		where = append(where, "bounty >= ?")
		args = append(args, q.MinBounty)
	}
	if q.MaxBounty > 0 {
		where = append(where, "bounty <= ?")
		args = append(args, q.MaxBounty)
	}
	if q.Publisher != "" {
		where = append(where, "publisher_id = ?")
		args = append(args, q.Publisher)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	orderBy := "created_at DESC"
	switch q.Sort {
	case "bounty_asc":
		orderBy = "bounty ASC"
	case "bounty_desc":
		orderBy = "bounty DESC"
	case "deadline_asc":
		orderBy = "deadline ASC"
	case "created_desc":
		orderBy = "created_at DESC"
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 || q.Limit > 100 {
		q.Limit = 20
	}

	offset := (q.Page - 1) * q.Limit
	queryArgs := append(args, q.Limit, offset)
	rows, err := s.db.Query(
		"SELECT id, publisher_id, title, description, requirements, bounty, escrow_tx, max_workers, payment_model, status, deadline, result, created_at, updated_at FROM tasks WHERE "+whereClause+" ORDER BY "+orderBy+" LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	return tasks, total, nil
}

func (s *SQLiteStore) UpdateTaskStatus(id string, status model.TaskStatus) error {
	_, err := s.db.Exec("UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?", status, time.Now(), id)
	return err
}

func (s *SQLiteStore) UpdateTaskResult(id, result string) error {
	_, err := s.db.Exec("UPDATE tasks SET result = ?, updated_at = ? WHERE id = ?", result, time.Now(), id)
	return err
}

func (s *SQLiteStore) GetTasksByPublisher(publisherID string) (int, int, error) {
	var published, completed int
	s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE publisher_id = ?", publisherID).Scan(&published)
	s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE publisher_id = ? AND status = 'completed'", publisherID).Scan(&completed)
	return published, completed, nil
}

// ==================== Bid ====================

func (s *SQLiteStore) CreateBid(b *model.Bid) error {
	_, err := s.db.Exec(
		`INSERT INTO bids (id, task_id, agent_id, proposal, price, estimated_tokens, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.TaskID, b.AgentID, b.Proposal, b.Price, b.EstimatedTokens, b.Status, b.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetBid(id string) (*model.Bid, error) {
	row := s.db.QueryRow(`SELECT id, task_id, agent_id, proposal, price, estimated_tokens, status, created_at FROM bids WHERE id = ?`, id)
	return scanBid(row)
}

func (s *SQLiteStore) GetBidsByTask(taskID string) ([]*model.Bid, error) {
	rows, err := s.db.Query(`SELECT b.id, b.task_id, b.agent_id, b.proposal, b.price, b.estimated_tokens, b.status, b.created_at FROM bids b WHERE b.task_id = ? ORDER BY b.created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bids []*model.Bid
	for rows.Next() {
		b := &model.Bid{}
		if err := rows.Scan(&b.ID, &b.TaskID, &b.AgentID, &b.Proposal, &b.Price, &b.EstimatedTokens, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		// look up agent name
		agent, _ := s.GetAgent(b.AgentID)
		if agent != nil {
			b.AgentName = agent.Name
		}
		bids = append(bids, b)
	}
	return bids, nil
}

func (s *SQLiteStore) UpdateBidStatus(id string, status model.BidStatus) error {
	_, err := s.db.Exec("UPDATE bids SET status = ? WHERE id = ?", status, id)
	return err
}

func (s *SQLiteStore) HasAgentBid(taskID, agentID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bids WHERE task_id = ? AND agent_id = ?", taskID, agentID).Scan(&count)
	return count > 0, err
}

// ==================== Task Workers ====================

func (s *SQLiteStore) AddTaskWorker(taskID, agentID string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO task_workers (task_id, agent_id) VALUES (?, ?)", taskID, agentID)
	return err
}

func (s *SQLiteStore) UpdateWorkerTokens(taskID, agentID string, prompt, completion int) error {
	_, err := s.db.Exec("UPDATE task_workers SET prompt_tokens = ?, completion_tokens = ? WHERE task_id = ? AND agent_id = ?", prompt, completion, taskID, agentID)
	return err
}

func (s *SQLiteStore) GetTaskWorkers(taskID string) ([]string, error) {
	rows, err := s.db.Query("SELECT agent_id FROM task_workers WHERE task_id = ?", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		workers = append(workers, id)
	}
	return workers, nil
}

func (s *SQLiteStore) IsTaskWorker(taskID, agentID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM task_workers WHERE task_id = ? AND agent_id = ?", taskID, agentID).Scan(&count)
	return count > 0, err
}

func (s *SQLiteStore) GetWorkerTokenUsage(taskID string) (map[string]model.TokenUsage, error) {
	rows, err := s.db.Query("SELECT agent_id, prompt_tokens, completion_tokens FROM task_workers WHERE task_id = ?", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usage := make(map[string]model.TokenUsage)
	for rows.Next() {
		var agentID string
		var tu model.TokenUsage
		if err := rows.Scan(&agentID, &tu.PromptTokens, &tu.CompletionTokens); err != nil {
			return nil, err
		}
		usage[agentID] = tu
	}
	return usage, nil
}

func (s *SQLiteStore) CountCompletedByWorker(agentID string) (int, int, error) {
	var total, completed int
	s.db.QueryRow("SELECT COUNT(*) FROM task_workers WHERE agent_id = ?", agentID).Scan(&total)
	s.db.QueryRow("SELECT COUNT(*) FROM task_workers tw JOIN tasks t ON tw.task_id = t.id WHERE tw.agent_id = ? AND t.status = 'completed'", agentID).Scan(&completed)
	return total, completed, nil
}

// ==================== Payment ====================

func (s *SQLiteStore) CreatePayment(p *model.Payment) error {
	_, err := s.db.Exec(
		`INSERT INTO payments (id, task_id, from_agent, to_agent, amount, platform_fee, tx_hash, type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.TaskID, p.FromAgent, p.ToAgent, p.Amount, p.PlatformFee, p.TxHash, p.Type, p.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetPaymentsByTask(taskID string) ([]*model.Payment, error) {
	rows, err := s.db.Query(`SELECT id, task_id, from_agent, to_agent, amount, platform_fee, tx_hash, type, created_at FROM payments WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPayments(rows)
}

func (s *SQLiteStore) ListPayments(agentID, payType string, page, limit int) ([]*model.Payment, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}

	if agentID != "" {
		where = append(where, "(from_agent = ? OR to_agent = ?)")
		args = append(args, agentID, agentID)
	}
	if payType == "earned" {
		where = append(where, "to_agent = ?")
		args = append(args, agentID)
	} else if payType == "spent" {
		where = append(where, "from_agent = ?")
		args = append(args, agentID)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM payments WHERE "+whereClause, args...).Scan(&total)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(
		"SELECT id, task_id, from_agent, to_agent, amount, platform_fee, tx_hash, type, created_at FROM payments WHERE "+whereClause+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	payments, err := scanPayments(rows)
	return payments, total, err
}

// ==================== Scanners ====================

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanAgent(row scanner) (*model.Agent, error) {
	a := &model.Agent{}
	var capsJSON string
	err := row.Scan(&a.ID, &a.Name, &a.Owner, &capsJSON, &a.WalletAddress, &a.APIKey,
		&a.Reputation, &a.TotalEarned, &a.TotalSpent, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(capsJSON), &a.Capabilities)
	return a, nil
}

func scanAgentRows(rows *sql.Rows) (*model.Agent, error) {
	a := &model.Agent{}
	var capsJSON string
	err := rows.Scan(&a.ID, &a.Name, &a.Owner, &capsJSON, &a.WalletAddress, &a.APIKey,
		&a.Reputation, &a.TotalEarned, &a.TotalSpent, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(capsJSON), &a.Capabilities)
	return a, nil
}

func scanTask(row scanner) (*model.Task, error) {
	t := &model.Task{}
	var reqsJSON string
	var result sql.NullString
	err := row.Scan(&t.ID, &t.PublisherID, &t.Title, &t.Description, &reqsJSON,
		&t.Bounty, &t.EscrowTx, &t.MaxWorkers, &t.PaymentModel,
		&t.Status, &t.Deadline, &result, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(reqsJSON), &t.Requirements)
	if result.Valid {
		t.Result = result.String
	}
	return t, nil
}

func scanTaskRows(rows *sql.Rows) (*model.Task, error) {
	t := &model.Task{}
	var reqsJSON string
	var result sql.NullString
	err := rows.Scan(&t.ID, &t.PublisherID, &t.Title, &t.Description, &reqsJSON,
		&t.Bounty, &t.EscrowTx, &t.MaxWorkers, &t.PaymentModel,
		&t.Status, &t.Deadline, &result, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(reqsJSON), &t.Requirements)
	if result.Valid {
		t.Result = result.String
	}
	return t, nil
}

func scanBid(row scanner) (*model.Bid, error) {
	b := &model.Bid{}
	err := row.Scan(&b.ID, &b.TaskID, &b.AgentID, &b.Proposal, &b.Price, &b.EstimatedTokens, &b.Status, &b.CreatedAt)
	return b, err
}

func scanPayments(rows *sql.Rows) ([]*model.Payment, error) {
	var payments []*model.Payment
	for rows.Next() {
		p := &model.Payment{}
		if err := rows.Scan(&p.ID, &p.TaskID, &p.FromAgent, &p.ToAgent, &p.Amount, &p.PlatformFee, &p.TxHash, &p.Type, &p.CreatedAt); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, nil
}
