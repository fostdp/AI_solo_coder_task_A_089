package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"plankroad-backend/monitoring"
)

func PrometheusMetrics(metrics *monitoring.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()

		metrics.SetActiveConns(1)

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		metrics.ObserveHTTPRequest(c.Request.Method, path, status, duration)
		metrics.SetActiveConns(-1)
	}
}
