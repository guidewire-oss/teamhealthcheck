// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupTeamDashboardRoutes registers team lead dashboard routes
// All routes require JWT authentication and team membership
func SetupTeamDashboardRoutes(router *gin.Engine, db *sql.DB, jwtService *services.JWTService) {
	handler := NewTeamDashboardHandler(db)

	// Team Lead Dashboard routes - require authentication + team membership
	dashboard := router.Group("/api/v1/teams/:teamId/dashboard")
	dashboard.Use(middleware.JWTAuthMiddleware(jwtService))
	dashboard.Use(middleware.TeamMembershipMiddleware("teamId"))
	{
		dashboard.GET("/health-summary", handler.GetHealthSummary)
		dashboard.GET("/response-distribution", handler.GetResponseDistribution)
		dashboard.GET("/individual-responses", handler.GetIndividualResponses)
		dashboard.GET("/trends", handler.GetTrends)
	}
}
