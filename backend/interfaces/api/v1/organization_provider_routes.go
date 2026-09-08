package v1

import (
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/gin-gonic/gin"
)

// SetupOrganizationProviderRoutes configures the external organization-data
// provider routes. Like every other admin route, these require a valid JWT
// and admin privileges.
//
// The settings endpoint joins the existing /api/v1/admin/settings group rather
// than introducing a second configuration surface. There is no PUT/update
// route: the provider credential is environment configuration, not something
// an admin enters through the UI.
func SetupOrganizationProviderRoutes(
	router *gin.Engine,
	syncService *services.OrganizationSyncService,
	jwtService *services.JWTService,
) {
	handler := NewOrganizationProviderHandler(syncService)

	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuthMiddleware(jwtService))
	admin.Use(middleware.AdminOnlyMiddleware())
	{
		admin.GET("/settings/organization-provider", handler.GetSettings)

		// Manual trigger. There is deliberately no scheduler: a sync can
		// hard-delete org structure, so a human decides when it happens.
		admin.POST("/organization-provider/sync", handler.Sync)
	}
}
