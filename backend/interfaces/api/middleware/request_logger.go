// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"time"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// RequestLoggerMiddleware logs HTTP requests with structured fields
// Logs all requests at appropriate levels based on status code:
// - 5xx: ERROR
// - 4xx: WARN
// - 2xx/3xx: INFO (can be filtered via log level config)
func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip health check endpoint to reduce noise
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/api/health" {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		status := c.Writer.Status()

		// Build log entry
		log := logger.Get()
		log.HTTP().
			Method(c.Request.Method).
			Path(path).
			Status(status).
			Duration(duration).
			IP(c.ClientIP()).
			RequestID(c.GetString("request_id")).
			Log()

		// Log query params at debug level if present (useful for debugging)
		if query != "" && zerolog.GlobalLevel() <= zerolog.DebugLevel {
			logger.Get().WithField("query", query).Debug("request query params")
		}
	}
}

// RequestIDMiddleware generates a unique request ID for tracing
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID is provided in header
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			// Generate a simple request ID (timestamp + counter for uniqueness)
			requestID = generateRequestID()
		}

		// Set in context for downstream use
		c.Set("request_id", requestID)

		// Set in response header for client correlation
		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()
	}
}

// Simple request ID generator using timestamp and atomic counter
var requestCounter uint64

func generateRequestID() string {
	// Use timestamp in milliseconds for rough ordering + simple counter
	// In production, consider using UUID or snowflake IDs
	requestCounter++
	return time.Now().Format("20060102150405") + "-" + string(rune('a'+requestCounter%26))
}
