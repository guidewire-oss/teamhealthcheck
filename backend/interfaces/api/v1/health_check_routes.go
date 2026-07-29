// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
)

// SetupHealthCheckRoutes registers health check routes with repository injection
// All routes require JWT authentication
func SetupHealthCheckRoutes(router *gin.Engine, healthCheckRepo healthcheck.Repository, orgRepo organization.Repository, jwtService *services.JWTService, notificationService *services.NotificationService) {
	handler := NewHealthCheckHandler(healthCheckRepo, orgRepo, notificationService)

	// Health check routes - all require authentication
	healthChecks := router.Group("/api/v1")
	healthChecks.Use(middleware.JWTAuthMiddleware(jwtService))
	{
		healthChecks.POST("/health-checks", handler.SubmitHealthCheck)
		healthChecks.GET("/health-dimensions", handler.GetHealthDimensions)
		healthChecks.GET("/health-checks/:id", handler.GetHealthCheckByID)
		// Using /health-checks/team/:id to avoid conflict with /teams/:id
		healthChecks.GET("/health-checks/team/:id", handler.GetTeamHealthChecks)

		// Team submission status for post-workshop surveys
		healthChecks.GET("/teams/:teamId/submission-status", handler.GetTeamSubmissionStatus)

		// Assessment periods (dynamic dropdown data)
		healthChecks.GET("/assessment-periods", handler.GetAssessmentPeriods)
	}
}
