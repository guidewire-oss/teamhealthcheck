// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// ValidationMiddleware provides input validation and sanitization
func ValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// ValidateRequest binds and validates JSON request body
// Returns true if validation passed, false otherwise (error response already sent)
func ValidateRequest(c *gin.Context, request interface{}) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		handleValidationError(c, err)
		return false
	}
	return true
}

// handleValidationError converts validation errors to user-friendly messages
func handleValidationError(c *gin.Context, err error) {
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		messages := make([]string, 0, len(validationErrors))
		for _, fieldErr := range validationErrors {
			messages = append(messages, formatValidationError(fieldErr))
		}
		dto.RespondError(c, http.StatusBadRequest, strings.Join(messages, "; "))
		return
	}
	dto.RespondError(c, http.StatusBadRequest, "Invalid request format")
}

// formatValidationError creates a user-friendly error message for a field
func formatValidationError(fieldErr validator.FieldError) string {
	field := fieldErr.Field()
	switch fieldErr.Tag() {
	case "required":
		return field + " is required"
	case "min":
		return field + " must be at least " + fieldErr.Param()
	case "max":
		return field + " must be at most " + fieldErr.Param()
	case "email":
		return field + " must be a valid email address"
	case "oneof":
		return field + " must be one of: " + fieldErr.Param()
	case "uuid":
		return field + " must be a valid UUID"
	case "alphanum":
		return field + " must contain only alphanumeric characters"
	default:
		return field + " is invalid"
	}
}

// SanitizeString removes potentially dangerous characters from strings
// Used to prevent XSS attacks in user-provided content
func SanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Basic HTML entity encoding for dangerous characters
	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(input)
}

// SanitizeComment sanitizes user comments while preserving basic formatting
func SanitizeComment(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Limit length
	if len(input) > 1000 {
		input = input[:1000]
	}

	// Encode HTML entities
	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(input)
}

// Validation patterns for common data types
var (
	uuidRegex     = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,50}$`)
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(input string) bool {
	return uuidRegex.MatchString(input)
}

// IsValidUsername checks if a string is a valid username
func IsValidUsername(input string) bool {
	return usernameRegex.MatchString(input)
}

// IsValidEmail checks if a string is a valid email
func IsValidEmail(input string) bool {
	return emailRegex.MatchString(input)
}

// ValidatePathParam validates a path parameter
func ValidatePathParam(c *gin.Context, param, validationType string) (string, bool) {
	value := c.Param(param)
	if value == "" {
		dto.RespondError(c, http.StatusBadRequest, param+" is required")
		return "", false
	}

	switch validationType {
	case "uuid":
		if !IsValidUUID(value) {
			dto.RespondError(c, http.StatusBadRequest, param+" must be a valid UUID")
			return "", false
		}
	case "username":
		if !IsValidUsername(value) {
			dto.RespondError(c, http.StatusBadRequest, param+" must be a valid username (2-50 alphanumeric characters, underscore, or hyphen)")
			return "", false
		}
	case "id":
		// Allow alphanumeric IDs with hyphens and underscores
		if len(value) < 1 || len(value) > 100 {
			dto.RespondError(c, http.StatusBadRequest, param+" must be between 1 and 100 characters")
			return "", false
		}
	}

	return value, true
}

// RateLimiter provides simple in-memory rate limiting
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int           // max requests
	window   time.Duration // time window
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request from the given key should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Clean old requests
	if times, exists := rl.requests[key]; exists {
		validTimes := make([]time.Time, 0, len(times))
		for _, t := range times {
			if t.After(windowStart) {
				validTimes = append(validTimes, t)
			}
		}
		rl.requests[key] = validTimes
	}

	// Check limit
	if len(rl.requests[key]) >= rl.limit {
		return false
	}

	// Add request
	rl.requests[key] = append(rl.requests[key], now)
	return true
}

// RateLimitMiddleware creates rate limiting middleware
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		// Use client IP as rate limit key
		clientIP := c.ClientIP()
		requestID := c.GetString("request_id")
		endpoint := c.Request.URL.Path

		if !limiter.Allow(clientIP) {
			log.Security("rate_limit_exceeded").
				IP(clientIP).
				RequestID(requestID).
				Endpoint(endpoint).
				Details("Client exceeded maximum allowed requests per time window").
				Log()
			dto.RespondError(c, http.StatusTooManyRequests, "Too many requests. Please try again later.")
			c.Abort()
			return
		}

		c.Next()
	}
}

// AuthRateLimiter is a stricter rate limiter for auth endpoints
var AuthRateLimiter = NewRateLimiter(10, time.Minute) // 10 requests per minute

// AuthRateLimitMiddleware provides rate limiting for auth endpoints
func AuthRateLimitMiddleware() gin.HandlerFunc {
	return RateLimitMiddleware(AuthRateLimiter)
}

// ContentTypeValidator ensures requests have the correct content type
func ContentTypeValidator() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only check POST, PUT, PATCH requests with body
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
			contentType := c.GetHeader("Content-Type")
			if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
				dto.RespondError(c, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// MaxBodySizeMiddleware limits request body size to prevent DoS attacks
func MaxBodySizeMiddleware(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}
