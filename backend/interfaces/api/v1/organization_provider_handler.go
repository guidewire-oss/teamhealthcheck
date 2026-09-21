package v1

import (
	"errors"
	"io"
	"math"
	"net/http"
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

// OrganizationProviderHandler serves the external organization-data provider
// configuration status and the manual synchronization trigger. It holds no
// credential and performs no persistence or mapping of its own -- both live
// in OrganizationSyncService and OrganizationProviderRepository respectively.
type OrganizationProviderHandler struct {
	syncService *services.OrganizationSyncService
	// settingsRepo persists the admin-configured mass-deletion threshold.
	// The threshold endpoints live here, next to the sync, because whether
	// they may run at all depends on the sync's own state.
	settingsRepo organization.Repository
	providerName string
}

// NewOrganizationProviderHandler creates the handler.
func NewOrganizationProviderHandler(
	syncService *services.OrganizationSyncService,
	settingsRepo organization.Repository,
) *OrganizationProviderHandler {
	return &OrganizationProviderHandler{
		syncService:  syncService,
		settingsRepo: settingsRepo,
		providerName: "data-provider",
	}
}

// lockState reports the current threshold lock, tolerating a nil sync service
// (which only happens in tests that exercise the settings endpoints alone).
func (h *OrganizationProviderHandler) lockState() services.SyncLockState {
	if h.syncService == nil {
		return services.SyncLockState{}
	}
	return h.syncService.LockState()
}

// thresholdResponse builds the settings body, including why the threshold is
// frozen and at what value, so a tab that has just loaded knows to disable its
// controls without a second request.
func (h *OrganizationProviderHandler) thresholdResponse(
	saved *float64,
	value float64,
	source string,
) dto.OrgSyncDeletionThreshold {
	body := dto.OrgSyncDeletionThreshold{
		MaxDeletePercent: value,
		Source:           source,
		DefaultPercent:   services.DefaultMaxDeletePercent,
		MinPercent:       services.MinConfigurableDeletePercent,
		MaxPercent:       services.MaxConfigurableDeletePercent,
	}

	state := h.lockState()
	body.Locked = state.Locked()
	switch {
	case state.Syncing:
		body.LockReason = dto.ThresholdLockSyncing
	case state.Held:
		body.LockReason = dto.ThresholdLockHeld
	}
	if body.Locked && state.Threshold > 0 {
		locked := state.Threshold
		body.ActiveSyncThreshold = &locked
	}
	return body
}

// GetDeletionThreshold handles
// GET /api/v1/admin/settings/organization-provider/deletion-threshold.
//
// It reports the threshold the guard would apply next, resolved by the same
// precedence the sync uses (saved admin setting, then
// ORG_SYNC_MAX_DELETE_PERCENT, then the built-in default), plus whether it is
// currently frozen by a running or held sync.
func (h *OrganizationProviderHandler) GetDeletionThreshold(c *gin.Context) {
	saved, err := h.settingsRepo.GetOrgSyncMaxDeletePercent(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch deletion threshold",
			Message: err.Error(),
		})
		return
	}

	value, source, err := services.ResolveMaxDeletePercent(saved)
	if err != nil {
		// A stored value outside the allowed range: report it as a
		// misconfiguration rather than pretending a different number applies.
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Configured deletion threshold is invalid",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, h.thresholdResponse(saved, value, source))
}

// UpdateDeletionThreshold handles
// PUT /api/v1/admin/settings/organization-provider/deletion-threshold.
//
// Only finite values from 1 through 100 are accepted, and only while no sync is
// running or held. That second rule is the substance of the guard: without it
// an admin could answer a mass-deletion hold by raising the limit and retrying,
// which would make the review step decorative. The one sanctioned way past a
// hold remains the explicit, authorized Sync Anyway override, which waives the
// threshold alone -- not validation, authorization, protected records, or the
// single-transaction guarantee.
func (h *OrganizationProviderHandler) UpdateDeletionThreshold(c *gin.Context) {
	// Checked before the body is even read, so a request from another tab is
	// refused on the same grounds as one from this tab.
	if state := h.lockState(); state.Locked() {
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:   "Deletion threshold is locked",
			Message: thresholdLockMessage(state),
			Code:    dto.CodeThresholdLocked,
		})
		return
	}

	var req dto.UpdateOrgSyncDeletionThresholdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
		return
	}

	if req.MaxDeletePercent == nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "maxDeletePercent is required",
			Message: "Provide maxDeletePercent as a finite number between 1 and 100",
		})
		return
	}

	percent := *req.MaxDeletePercent
	// NaN and Inf are rejected explicitly: both slip past a range comparison.
	if math.IsNaN(percent) || math.IsInf(percent, 0) ||
		percent < services.MinConfigurableDeletePercent || percent > services.MaxConfigurableDeletePercent {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "maxDeletePercent must be between 1 and 100",
			Message: "maxDeletePercent must be a finite number between 1 and 100",
		})
		return
	}

	// Re-checked after validation and immediately before the write: a sync that
	// started while this request was being parsed must not have its threshold
	// moved out from under it.
	if state := h.lockState(); state.Locked() {
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:   "Deletion threshold is locked",
			Message: thresholdLockMessage(state),
			Code:    dto.CodeThresholdLocked,
		})
		return
	}

	if err := h.settingsRepo.UpdateOrgSyncMaxDeletePercent(c.Request.Context(), percent); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to save deletion threshold",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, h.thresholdResponse(&percent, percent, services.ThresholdSourceAdmin))
}

// DismissMassDeletionHold handles
// DELETE /api/v1/admin/organization-provider/sync/hold.
//
// It resolves a hold the administrator has decided not to override -- because
// the provider data is what needs fixing -- and so unfreezes the threshold. It
// applies nothing and deletes nothing: the held sync stays unapplied.
func (h *OrganizationProviderHandler) DismissMassDeletionHold(c *gin.Context) {
	if h.syncService == nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "There is no active synchronization or hold to dismiss",
		})
		return
	}

	// A hold cannot be dismissed out from under a running sync.
	if h.syncService.LockState().Syncing {
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:   "A synchronization is already running",
			Message: "Wait for the running synchronization to finish, then try again.",
			Code:    dto.CodeThresholdLocked,
		})
		return
	}

	h.syncService.DismissHold()

	saved, err := h.settingsRepo.GetOrgSyncMaxDeletePercent(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch deletion threshold",
			Message: err.Error(),
		})
		return
	}
	value, source, err := services.ResolveMaxDeletePercent(saved)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Configured deletion threshold is invalid",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, h.thresholdResponse(saved, value, source))
}

// thresholdLockMessage explains a refusal in the terms the admin is looking at.
func thresholdLockMessage(state services.SyncLockState) string {
	if state.Syncing {
		return "A synchronization is running. The threshold cannot be changed until it finishes."
	}
	return "A synchronization is held for mass-deletion review. Resolve that hold -- apply it with Sync Anyway, or dismiss it -- before changing the threshold."
}

// GetSettings handles GET /api/v1/admin/settings/organization-provider.
//
// This reports environment-configuration readiness only. It never queries a
// credential table (there is none) and never returns a token value.
func (h *OrganizationProviderHandler) GetSettings(c *gin.Context) {
	baseURL := os.Getenv(dataprovider.EnvBaseURL)
	settings := dto.OrganizationProviderSettingsDTO{
		Provider:          h.providerName,
		BaseURLConfigured: baseURL != "" && dataprovider.ValidateBaseURL(baseURL) == nil,
		TokenConfigured:   os.Getenv(dataprovider.EnvAPIToken) != "",
		ReadyToSync:       h.syncService != nil && h.syncService.Configured(),
	}

	c.JSON(http.StatusOK, settings)
}

// Sync handles POST /api/v1/admin/organization-provider/sync.

func (h *OrganizationProviderHandler) Sync(c *gin.Context) {
	if h.syncService == nil || !h.syncService.Configured() {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "Set " + dataprovider.EnvBaseURL + " and " + dataprovider.EnvAPIToken + " on the API service",
		})
		return
	}

	var request dto.OrganizationSyncRequestDTO
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   "Invalid request body",
				Message: `Expected an empty body or {"overrideMassDeletion": true, "confirmedMassDeletion": {...}}`,
			})
			return
		}
	}

	actorID, _ := middleware.GetUserIDFromContext(c)

	if request.OverrideMassDeletion && !isAdminRequest(c) {
		logger.Get().Security("org_sync_mass_deletion_override_denied").
			UserID(actorID).
			IP(c.ClientIP()).
			Endpoint(c.Request.URL.Path).
			Details("non-admin requested a mass-deletion override").
			Log()
		c.JSON(http.StatusForbidden, dto.ErrorResponse{
			Error:   "Access denied: admin privileges required",
			Message: "Overriding the mass-deletion hold requires an administrator",
		})
		return
	}

	// The override must be bound to the exact counts the admin reviewed, not
	// just a bare flag -- every sync fetches its own fresh snapshot, so a bare
	// flag would waive the guard for whatever that fresh fetch turns up, which
	// may no longer be what was shown. A request missing the confirmation is
	// rejected outright rather than silently falling back to a held sync, so
	// the caller finds out immediately that its request was malformed.
	var confirmed *orgprovider.ConfirmedMassDeletion
	if request.OverrideMassDeletion {
		if request.ConfirmedMassDeletion == nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   "Missing confirmation",
				Message: "overrideMassDeletion requires confirmedMassDeletion, echoing the counts from the held sync's response",
			})
			return
		}
		confirmed = &orgprovider.ConfirmedMassDeletion{
			UsersExisting: request.ConfirmedMassDeletion.UsersExisting,
			UsersDeleting: request.ConfirmedMassDeletion.UsersDeleting,
			TeamsExisting: request.ConfirmedMassDeletion.TeamsExisting,
			TeamsDeleting: request.ConfirmedMassDeletion.TeamsDeleting,
		}
	}

	result, err := h.syncService.SyncWithOptions(c.Request.Context(), services.SyncOptions{
		OverrideMassDeletion:  request.OverrideMassDeletion,
		ConfirmedMassDeletion: confirmed,
		ActorUserID:           actorID,
	})
	if err != nil {
		status, response := mapSyncError(err)
		c.JSON(status, response)
		return
	}

	c.JSON(http.StatusOK, result)
}

// isAdminRequest re-derives admin status from the validated JWT claims already
// on the context. It mirrors AdminOnlyMiddleware's rule rather than inventing a
// second one, and exists so the override can never be authorized by anything
// the client sent in the request body.
func isAdminRequest(c *gin.Context) bool {
	level, exists := c.Get("hierarchyLevel")
	if !exists {
		return false
	}
	asString, ok := level.(string)
	return ok && (asString == "level-1" || asString == "level-admin")
}

// mapSyncError converts a sync failure into a status code and a safe body.
// None of these branches echo a token, header, or the raw provider payload --
// including the mass-deletion hold, which carries aggregate counts only.
func mapSyncError(err error) (int, any) {
	switch {
	case errors.Is(err, services.ErrSyncInProgress):
		return http.StatusConflict, dto.ErrorResponse{
			Error:   "A synchronization is already running",
			Message: "Another synchronization is already in progress. Wait for it to finish, then try again.",
		}

	case errors.Is(err, services.ErrProviderNotConfigured):
		return http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "Set " + dataprovider.EnvBaseURL + " and " + dataprovider.EnvAPIToken + " on the API service",
		}

	case errors.Is(err, services.ErrMaxDeletePercentNotConfigured):
		return http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Mass-deletion guard is misconfigured",
			Message: "The saved mass-deletion threshold is outside the allowed 1-100% range. Correct it in Admin Settings, then re-run the sync.",
		}

	case errors.Is(err, services.ErrInvalidSnapshot):
		return http.StatusBadGateway, dto.ErrorResponse{
			Error:   "Provider returned data that failed contract validation",
			Message: err.Error(),
		}

	case errors.Is(err, services.ErrProviderFetchFailed):
		// The provider failed or returned an unusable response, so return 502 for this upstream failure.
		return http.StatusBadGateway, dto.ErrorResponse{
			Error:   "Failed to reach the organization data provider",
			Message: err.Error(),
		}

	case errors.Is(err, orgprovider.ErrMassDeletionBlocked):
		response := dto.MassDeletionHoldResponseDTO{
			ErrorResponse: dto.ErrorResponse{
				Error:   "Synchronization held for review",
				Message: "This sync would remove an unusually large share of users or teams. No changes were made. Review the incoming provider data, then either fix it upstream or re-run the sync with an explicit override.",
				Code:    dto.CodeMassDeletionHold,
			},
			Applied: false,
		}
		var hold *orgprovider.MassDeletionHoldError
		if errors.As(err, &hold) {
			response.MassDeletion = massDeletionReportDTO(hold.Report)
		}
		return http.StatusConflict, response

	default:
		// Service-side failures are internal errors, so return 500 rather than 502.
		logger.Get().WithError(err).Error("organization provider sync failed")
		return http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Synchronization failed",
			Message: err.Error(),
		}
	}
}

// massDeletionReportDTO maps the domain report onto the transport shape. The
// dto package imports no domain package, so the translation lives here.
func massDeletionReportDTO(report orgprovider.MassDeletionReport) *dto.MassDeletionReportDTO {
	out := &dto.MassDeletionReportDTO{
		Threshold: report.Threshold,
		Users:     deletionMetricDTO(report.Users),
		Teams:     deletionMetricDTO(report.Teams),
	}
	if report.Memberships != nil {
		memberships := deletionMetricDTO(*report.Memberships)
		out.Memberships = &memberships
	}
	return out
}

func deletionMetricDTO(metric orgprovider.DeletionMetric) dto.DeletionMetricDTO {
	return dto.DeletionMetricDTO{
		Kind:              metric.Kind,
		Existing:          metric.Existing,
		Incoming:          metric.Incoming,
		Deleting:          metric.Deleting,
		Percent:           metric.Percent,
		Threshold:         metric.Threshold,
		ExceedsThreshold:  metric.ExceedsThreshold,
		ContributesToHold: metric.ContributesToHold,
	}
}
