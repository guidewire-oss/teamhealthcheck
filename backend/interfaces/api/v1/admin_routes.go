// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupAdminRoutes configures admin routes with repository dependency injection
// All admin routes require JWT authentication and admin privileges (level-1)
func SetupAdminRoutes(router *gin.Engine, orgRepo organization.Repository, userRepo user.Repository, teamRepo team.Repository, jwtService *services.JWTService) {
	handler := NewAdminHandler(orgRepo, userRepo, teamRepo)

	admin := router.Group("/api/v1/admin")
	// Apply JWT authentication and admin-only authorization to all admin routes
	admin.Use(middleware.JWTAuthMiddleware(jwtService))
	admin.Use(middleware.AdminOnlyMiddleware())
	{
		// Hierarchy Levels CRUD
		hierarchyLevels := admin.Group("/hierarchy-levels")
		{
			hierarchyLevels.GET("", handler.ListHierarchyLevels)
			hierarchyLevels.POST("", handler.CreateHierarchyLevel)
			hierarchyLevels.PUT("/:id", handler.UpdateHierarchyLevel)
			hierarchyLevels.PUT("/:id/position", handler.UpdateHierarchyPosition)
			hierarchyLevels.DELETE("/:id", handler.DeleteHierarchyLevel)
		}

		// Users CRUD
		users := admin.Group("/users")
		{
			users.GET("", handler.ListUsers)
			users.POST("", handler.CreateUser)
			users.PUT("/:id", handler.UpdateUser)
			users.DELETE("/:id", handler.DeleteUser)
		}

		// Teams CRUD
		teams := admin.Group("/teams")
		{
			teams.GET("", handler.ListTeams)
			teams.POST("", handler.CreateTeam)
			teams.PUT("/:id", handler.UpdateTeam)
			teams.DELETE("/:id", handler.DeleteTeam)
			teams.GET("/:id/members", handler.GetTeamMembers)
			teams.POST("/:id/members", handler.AddTeamMember)
			teams.DELETE("/:id/members/:userId", handler.RemoveTeamMember)
			teams.GET("/:id/supervisors", handler.GetTeamSupervisors)
			teams.PUT("/:id/supervisors", handler.UpdateTeamSupervisors)
		}

		// Settings
		settings := admin.Group("/settings")
		{
			// Health Dimensions - Full CRUD
			settings.GET("/dimensions", handler.GetDimensions)
			settings.POST("/dimensions", handler.CreateDimension)
			settings.PUT("/dimensions/:id", handler.UpdateDimension)
			settings.DELETE("/dimensions/:id", handler.DeleteDimension)

			// Branding
			settings.GET("/branding", handler.GetBrandingSettings)
			settings.PUT("/branding", handler.UpdateBrandingSettings)

			// Notifications
			settings.GET("/notifications", handler.GetNotificationSettings)
			settings.PUT("/notifications", handler.UpdateNotificationSettings)

			// Retention Policy
			settings.GET("/retention", handler.GetRetentionPolicy)
			settings.PUT("/retention", handler.UpdateRetentionPolicy)
		}
	}
}
