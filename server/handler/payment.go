package handler

import (
	"net/http"
	"strconv"

	"github.com/clawwork/server/service"
	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	svc *service.PaymentService
}

func NewPaymentHandler(svc *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

func (h *PaymentHandler) GetEscrow(c *gin.Context) {
	taskID := c.Param("task_id")
	info, err := h.svc.GetEscrowStatus(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": 404, "message": "Task not found"}})
		return
	}

	c.JSON(http.StatusOK, info)
}

func (h *PaymentHandler) History(c *gin.Context) {
	agentID := c.Query("agent_id")
	payType := c.DefaultQuery("type", "all")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	// If no agent_id specified, use the caller's
	if agentID == "" {
		agentID = c.GetString("agent_id")
	}

	payments, total, err := h.svc.ListHistory(agentID, payType, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payments": payments,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}
