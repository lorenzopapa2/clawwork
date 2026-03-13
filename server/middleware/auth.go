package middleware

import (
	"net/http"

	"github.com/clawwork/server/store"
	"github.com/gin-gonic/gin"
)

func APIKeyAuth(db *store.SQLiteStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    401,
					"message": "Missing API key",
					"details": "Provide X-API-Key header",
				},
			})
			c.Abort()
			return
		}

		agent, err := db.GetAgentByAPIKey(apiKey)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    401,
					"message": "Invalid API key",
				},
			})
			c.Abort()
			return
		}

		c.Set("agent_id", agent.ID)
		c.Set("agent", agent)
		c.Next()
	}
}
