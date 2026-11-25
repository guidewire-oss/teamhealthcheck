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
	opts ...OrganizationProviderRouteOption,
) {
	var cfg organizationProviderRouteConfig
	for _, o := range opts {
		o(&cfg)
	}
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

		// Manual trigger. Automatic syncs are configured separately (auto-sync
		// below) and run the same StartRun path with the same guards.
		admin.POST("/organization-provider/sync", handler.Sync)
		admin.GET("/organization-provider/sync/runs/:runId", handler.GetSyncRun)
		// Read-only, paginated run history. Gin matches the static "runs"
		// path separately from the ":runId" route above.
		admin.GET("/organization-provider/sync/runs", handler.ListSyncRunHistory)

		if cfg.scheduler != nil {
			sched := &OrganizationSyncScheduleHandler{scheduler: cfg.scheduler}
			admin.GET("/organization-provider/auto-sync", sched.GetAutoSync)
			admin.PUT("/organization-provider/auto-sync", sched.UpdateAutoSync)
		}
	}
}

type organizationProviderRouteConfig struct {
	scheduler *services.OrganizationSyncScheduler
}

// OrganizationProviderRouteOption customises SetupOrganizationProviderRoutes.
type OrganizationProviderRouteOption func(*organizationProviderRouteConfig)

// WithAutoSyncScheduler mounts the admin-only auto-sync schedule endpoints.
func WithAutoSyncScheduler(s *services.OrganizationSyncScheduler) OrganizationProviderRouteOption {
	return func(c *organizationProviderRouteConfig) { c.scheduler = s }
}
