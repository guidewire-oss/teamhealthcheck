package v1

import (
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupActionItemRoutes registers action item routes
func SetupActionItemRoutes(router *gin.Engine, db *sql.DB, jwtService *services.JWTService) {
	handler := NewActionItemHandler(db)

	// Team-scoped routes — require JWT + team membership
	teamRoutes := router.Group("/api/v1/teams/:teamId/action-items")
	teamRoutes.Use(middleware.JWTAuthMiddleware(jwtService))
	teamRoutes.Use(middleware.TeamMembershipMiddleware("teamId"))
	{
		teamRoutes.GET("", handler.ListActionItems)
		teamRoutes.POST("", handler.CreateActionItem)
		teamRoutes.PATCH("/:id", handler.UpdateActionItem)
		teamRoutes.DELETE("/:id", handler.DeleteActionItem)
	}

	// Manager summary route — requires JWT + manager or above role
	managerRoutes := router.Group("/api/v1/managers/:managerId/teams/action-items")
	managerRoutes.Use(middleware.JWTAuthMiddleware(jwtService))
	managerRoutes.Use(middleware.ManagerOrAboveMiddleware())
	{
		managerRoutes.GET("", handler.GetTeamsActionSummary)
	}
}
