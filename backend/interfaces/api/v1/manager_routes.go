// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupManagerRoutes registers manager-related routes with repository dependency injection
// All manager routes require JWT authentication and manager or above privileges
func SetupManagerRoutes(router *gin.Engine, healthCheckRepo healthcheck.Repository, trendsService *trends.Service, jwtService *services.JWTService, userRepo user.Repository) {
	handler := NewManagerHandler(healthCheckRepo, trendsService, userRepo)

	// Manager dashboard routes - require authentication and manager+ role
	managers := router.Group("/api/v1/managers")
	managers.Use(middleware.JWTAuthMiddleware(jwtService))
	managers.Use(middleware.ManagerOrAboveMiddleware())
	managers.Use(middleware.SameUserOrManagerMiddleware("managerId")) // Ensure users can only access their own data
	{
		managers.GET("/:managerId/teams/health", handler.GetManagerTeamsHealth)
		managers.GET("/:managerId/dashboard/radar", handler.GetManagerAggregatedRadar)
		managers.GET("/:managerId/dashboard/trends", handler.GetManagerTrends)
		managers.GET("/:managerId/subordinates", handler.GetSubordinates)
	}
}
