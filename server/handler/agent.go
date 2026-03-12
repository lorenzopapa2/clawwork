package handler

import (
	"net/http"
	"strconv"

	"github.com/agenthub/server/model"
	"github.com/agenthub/server/service"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

func (h *AgentHandler) Register(c *gin.Context) {
	var req model.RegisterAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	agent, err := h.svc.Register(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	c.JSON(http.StatusCreated, agent)
}

func (h *AgentHandler) List(c *gin.Context) {
	status := c.Query("status")
	capability := c.Query("capability")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	agents, total, err := h.svc.List(status, capability, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	// Strip API keys from list response
	for _, a := range agents {
		a.APIKey = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

func (h *AgentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	agent, err := h.svc.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": 404, "message": "Agent not found"}})
		return
	}

	// Only show API key to the agent itself
	callerID, _ := c.Get("agent_id")
	if callerID != agent.ID {
		agent.APIKey = ""
	}

	c.JSON(http.StatusOK, agent)
}

func (h *AgentHandler) Update(c *gin.Context) {
	id := c.Param("id")
	callerID := c.GetString("agent_id")
	if callerID != id {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": 403, "message": "Cannot update another agent"}})
		return
	}

	var req model.UpdateAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": 400, "message": "Invalid request", "details": err.Error()}})
		return
	}

	if err := h.svc.Update(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": 500, "message": err.Error()}})
		return
	}

	agent, _ := h.svc.Get(id)
	c.JSON(http.StatusOK, agent)
}

func (h *AgentHandler) Stats(c *gin.Context) {
	id := c.Param("id")
	stats, err := h.svc.GetStats(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": 404, "message": "Agent not found"}})
		return
	}

	c.JSON(http.StatusOK, stats)
}
