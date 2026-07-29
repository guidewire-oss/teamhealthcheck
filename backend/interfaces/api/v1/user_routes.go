// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupUserRoutes registers user-related routes
// All routes require JWT authentication
func SetupUserRoutes(router *gin.Engine, db *sql.DB, jwtService *services.JWTService) {
	handler := NewUserHandler(db)

	// User routes - require authentication + same user or manager
	userRoutes := router.Group("/api/v1/users/:userId")
	userRoutes.Use(middleware.JWTAuthMiddleware(jwtService))
	userRoutes.Use(middleware.SameUserOrManagerMiddleware("userId"))
	{
		userRoutes.GET("/survey-history", handler.GetUserSurveyHistory)
	}
}
