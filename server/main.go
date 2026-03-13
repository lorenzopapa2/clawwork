package main

import (
	"log"
	"net/http"

	"github.com/clawwork/server/config"
	"github.com/clawwork/server/handler"
	"github.com/clawwork/server/middleware"
	"github.com/clawwork/server/service"
	"github.com/clawwork/server/store"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	// Initialize store
	db, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize blockchain service (gracefully disabled if not configured)
	blockchainSvc := service.NewBlockchainService(cfg)

	// Initialize services
	agentSvc := service.NewAgentService(db)
	taskSvc := service.NewTaskService(db, blockchainSvc)
	paymentSvc := service.NewPaymentService(db, blockchainSvc)
	matcherSvc := service.NewMatcherService(db)

	// Initialize handlers
	agentH := handler.NewAgentHandler(agentSvc)
	taskH := handler.NewTaskHandler(taskSvc, matcherSvc)
	paymentH := handler.NewPaymentHandler(paymentSvc)

	// Setup router
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"version":    "1.0.0",
			"blockchain": blockchainSvc.IsEnabled(),
		})
	})

	// Public routes (no auth required)
	v1 := r.Group("/api/v1")
	{
		v1.POST("/agents/register", agentH.Register)
		v1.GET("/agents", agentH.List)
		v1.GET("/agents/:id", agentH.Get)
		v1.GET("/agents/:id/stats", agentH.Stats)
	}

	// Authenticated routes
	auth := v1.Group("")
	auth.Use(middleware.APIKeyAuth(db))
	{
		// Agent management
		auth.PUT("/agents/:id", agentH.Update)

		// Task market
		auth.POST("/tasks", taskH.Create)
		auth.GET("/tasks", taskH.List)
		auth.GET("/tasks/:id", taskH.Get)
		auth.POST("/tasks/:id/bid", taskH.Bid)
		auth.PUT("/tasks/:id/assign", taskH.Assign)
		auth.PUT("/tasks/:id/submit", taskH.Submit)
		auth.PUT("/tasks/:id/approve", taskH.Approve)
		auth.PUT("/tasks/:id/dispute", taskH.Dispute)
		auth.GET("/tasks/:id/match", taskH.MatchAgents)

		// Payments
		auth.GET("/payments/escrow/:task_id", paymentH.GetEscrow)
		auth.GET("/payments/history", paymentH.History)
	}

	// Serve static web dashboard
	r.StaticFile("/", "../web/index.html")
	r.StaticFile("/index.html", "../web/index.html")

	log.Printf("ClawWork server starting on :%s", cfg.Port)
	if blockchainSvc.IsEnabled() {
		log.Printf("Blockchain integration: ENABLED")
	} else {
		log.Printf("Blockchain integration: DISABLED (set CONTRACT_ADDR and PLATFORM_PRIVATE_KEY to enable)")
	}

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
