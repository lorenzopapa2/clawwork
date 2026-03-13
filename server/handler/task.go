package handler

import (
	"net/http"

	"github.com/clawwork/server/model"
	"github.com/clawwork/server/service"
	"github.com/gin-gonic/gin"
)

type TaskHandler struct {
	taskSvc    *service.TaskService
	matcherSvc *service.MatcherService
}

func NewTaskHandler(taskSvc *service.TaskService, matcherSvc *service.MatcherService) *TaskHandler {
	return &TaskHandler{taskSvc: taskSvc, matcherSvc: matcherSvc}
}

func (h *TaskHandler) Create(c *gin.Context) {
	var req model.CreateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	publisherID := c.GetString("agent_id")
	task, err := h.taskSvc.Create(publisherID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) List(c *gin.Context) {
	var q model.TaskListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid query", "details": err.Error()}})
		return
	}

	tasks, total, err := h.taskSvc.List(&q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"total": total,
		"page":  q.Page,
		"limit": q.Limit,
	})
}

func (h *TaskHandler) Get(c *gin.Context) {
	id := c.Param("id")
	task, err := h.taskSvc.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": 404, "message": "Task not found"}})
		return
	}

	// Include bids
	bids, _ := h.taskSvc.GetBids(id)

	c.JSON(http.StatusOK, gin.H{
		"id":            task.ID,
		"publisher_id":  task.PublisherID,
		"title":         task.Title,
		"description":   task.Description,
		"requirements":  task.Requirements,
		"bounty":        task.Bounty,
		"escrow_tx":     task.EscrowTx,
		"max_workers":   task.MaxWorkers,
		"payment_model": task.PaymentModel,
		"status":        task.Status,
		"deadline":      task.Deadline,
		"bids":          bids,
		"result":        task.Result,
		"created_at":    task.CreatedAt,
		"updated_at":    task.UpdatedAt,
	})
}

func (h *TaskHandler) Bid(c *gin.Context) {
	taskID := c.Param("id")
	agentID := c.GetString("agent_id")

	var req model.BidReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	bid, err := h.taskSvc.Bid(taskID, agentID, &req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": 409, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusCreated, bid)
}

func (h *TaskHandler) Assign(c *gin.Context) {
	taskID := c.Param("id")
	publisherID := c.GetString("agent_id")

	var req model.AssignReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	if err := h.taskSvc.Assign(taskID, publisherID, &req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": 409, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status":  "assigned",
	})
}

func (h *TaskHandler) Submit(c *gin.Context) {
	taskID := c.Param("id")
	agentID := c.GetString("agent_id")

	var req model.SubmitResultReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	if err := h.taskSvc.Submit(taskID, agentID, &req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": 409, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status":  "review",
	})
}

func (h *TaskHandler) Approve(c *gin.Context) {
	taskID := c.Param("id")
	publisherID := c.GetString("agent_id")

	var req model.ApproveReq
	c.ShouldBindJSON(&req) // optional body

	distributions, err := h.taskSvc.Approve(taskID, publisherID, &req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": 409, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":       taskID,
		"status":        "completed",
		"distributions": distributions,
	})
}

func (h *TaskHandler) Dispute(c *gin.Context) {
	taskID := c.Param("id")
	agentID := c.GetString("agent_id")

	var req model.DisputeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	if err := h.taskSvc.Dispute(taskID, agentID, &req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": 409, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status":  "disputed",
	})
}

func (h *TaskHandler) MatchAgents(c *gin.Context) {
	taskID := c.Param("id")
	task, err := h.taskSvc.Get(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": 404, "message": "Task not found"}})
		return
	}

	results, err := h.matcherSvc.FindMatchingAgents(task, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"matches": results})
}
