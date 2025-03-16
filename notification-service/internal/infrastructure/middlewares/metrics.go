package middlewares

import (
	"net/http"
	"time"

	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/metrics"
	"github.com/gin-gonic/gin"
)

// MetricsMiddleware returns a Gin middleware function that records HTTP request durations.
func MetricsMiddleware(m metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()

		method := c.Request.Method
		status := http.StatusText(c.Writer.Status())

		m.ObserveHTTPRequestDuration(c.FullPath(), method, status, duration)
	}
}
