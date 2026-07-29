// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package dto

import (
	"github.com/gin-gonic/gin"
)

// RespondSuccess sends a successful JSON response with the provided data
// Use this for endpoints that return a single object or structured data
func RespondSuccess(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// RespondError sends an error JSON response with the standard error structure
// Always uses {"error": "message"} format for consistency
func RespondError(c *gin.Context, status int, message string) {
	c.JSON(status, ErrorResponse{
		Error: message,
	})
}

// RespondErrorWithDetails sends an error with additional details
func RespondErrorWithDetails(c *gin.Context, status int, message string, details string) {
	c.JSON(status, ErrorResponse{
		Error:   message,
		Message: details,
	})
}

// RespondList sends a successful JSON response for list endpoints
// Wraps items in the specified key with a total count
func RespondList(c *gin.Context, status int, items interface{}, total int) {
	c.JSON(status, gin.H{
		"items": items,
		"total": total,
	})
}

// RespondMessage sends a simple success message
// Use this for operations that don't return data (e.g., delete)
func RespondMessage(c *gin.Context, status int, message string) {
	c.JSON(status, MessageResponse{
		Message: message,
	})
}

// MessageResponse represents a simple message response
type MessageResponse struct {
	Message string `json:"message"`
}
