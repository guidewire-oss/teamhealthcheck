package v1

import (
	"time"

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
//
// scheduleLoc is the IANA location the schedule endpoint computes
// occurrences in when an admin enables/re-enables it -- must not be nil,
// pass time.UTC where no other zone applies.
func SetupOrganizationProviderRoutes(
	router *gin.Engine,
	syncService *services.OrganizationSyncService,
	settingsRepo organization.Repository,
	jwtService *services.JWTService,
	scheduleLoc *time.Location,
) {
	handler := NewOrganizationProviderHandler(syncService, settingsRepo, scheduleLoc)

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

		// Manual trigger.
		admin.POST("/organization-provider/sync", handler.Sync)

		// Automatic schedule. Disabled by default; when enabled, runs through
		// the exact same OrganizationSyncService.SyncWithOptions path as the
		// manual trigger above -- see backend/application/scheduler. A sync
		// can hard-delete org structure, so the schedule is admin-configured
		// and off until explicitly turned on, never auto-approves a mass
		// deletion, and is safe across replicas via the same advisory lock
		// the manual path uses.
		admin.GET("/settings/organization-provider/schedule", handler.GetOrgSyncSchedule)
		admin.PUT("/settings/organization-provider/schedule", handler.UpdateOrgSyncSchedule)

		// Persisted last-attempt result, for display after refresh,
		// logout/login, or a process restart.
		admin.GET("/organization-provider/sync/last-run", handler.GetOrgSyncLastRun)

		// Append-only history of every sync outcome (manual and scheduled;
		// success, blocked, failed, skipped), newest first. Each entry's runId
		// matches the sync_run_id on that attempt's log lines.
		admin.GET("/organization-provider/sync/history", handler.GetOrgSyncHistory)
	}
}
