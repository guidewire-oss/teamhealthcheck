package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupOrganizationProviderRoutes configures the external organization-data
// provider routes. Like every other admin route, these require a valid JWT
// and admin privileges.
//
// The settings endpoints join the existing /api/v1/admin/settings group rather
// than introducing a second configuration surface. There is still no route for
// the provider credential: that is environment configuration, not something an
// admin enters through the UI. The mass-deletion threshold is different -- it
// is a safety percentage, and it is served here rather than alongside the other
// admin settings because whether it may be changed depends on this sync's own
// running/held state.
func SetupOrganizationProviderRoutes(
	router *gin.Engine,
	syncService *services.OrganizationSyncService,
	settingsRepo organization.Repository,
	jwtService *services.JWTService,
) {
	handler := NewOrganizationProviderHandler(syncService, settingsRepo)

	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(jwtService))
	admin.Use(middleware.AdminOnlyMiddleware())
	{
		admin.GET("/settings/organization-provider", handler.GetSettings)

		// Mass-deletion protection. Reads report the lock state too, so a tab
		// that has just loaded knows whether editing is allowed.
		admin.GET("/settings/organization-provider/deletion-threshold", handler.GetDeletionThreshold)
		admin.PUT("/settings/organization-provider/deletion-threshold", handler.UpdateDeletionThreshold)

		// Resolves a hold without applying it, which unfreezes the threshold.
		admin.DELETE("/organization-provider/sync/hold", handler.DismissMassDeletionHold)

		// Manual trigger. There is deliberately no scheduler: a sync can
		// hard-delete org structure, so a human decides when it happens.
		admin.POST("/organization-provider/sync", handler.Sync)
	}
}
