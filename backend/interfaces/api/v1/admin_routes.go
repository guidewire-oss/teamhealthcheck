package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/email"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// AdminOption customizes SetupAdminRoutes.
type AdminOption func(*AdminHandler)

// WithReminderSender sets the email transport survey reminders go out
// through. Without it the reminders endpoint answers 503.
func WithReminderSender(sender email.Sender) AdminOption {
	return func(h *AdminHandler) {
		h.SurveyCompletionHandler.reminderSender = sender
	}
}

// WithReminderTestPod limits survey reminders to one pod's team lead and
// members (EMAIL_TEST_POD). An empty name leaves reminders unrestricted.
func WithReminderTestPod(podName string) AdminOption {
	return func(h *AdminHandler) {
		h.SurveyCompletionHandler.reminderTestPod = podName
	}
}

// SetupAdminRoutes configures admin routes with repository dependency injection
// All admin routes require JWT authentication and admin privileges (level-1)
func SetupAdminRoutes(router *gin.Engine, orgRepo organization.Repository, userRepo user.Repository, teamRepo team.Repository, healthCheckRepo healthcheck.Repository, jwtService *services.JWTService, opts ...AdminOption) {
	handler := NewAdminHandler(orgRepo, userRepo, teamRepo, healthCheckRepo)
	for _, opt := range opts {
		opt(handler)
	}

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
			users.GET("/lite", handler.ListUsersLite)
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

		// Survey completion dashboard (org-wide, all teams grouped by leader)
		admin.GET("/survey-completion", handler.GetSurveyCompletion)
		// Emails the pods that are still in progress or not started.
		admin.POST("/survey-completion/reminders", handler.SendSurveyReminders)
	}
}
