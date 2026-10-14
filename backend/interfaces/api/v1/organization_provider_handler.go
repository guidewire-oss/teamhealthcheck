package v1

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/scheduler"
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/interfaces/middleware"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrganizationProviderHandler serves the external organization-data provider
// configuration status and the manual synchronization trigger. It holds no
// credential and performs no persistence or mapping of its own -- both live
// in OrganizationSyncService and OrganizationProviderRepository respectively.
type OrganizationProviderHandler struct {
	syncService *services.OrganizationSyncService
	// settingsRepo persists the admin-configured mass-deletion threshold and
	// the automatic-sync schedule. The threshold endpoints live here because
	// whether the threshold may be changed depends on the sync's own
	// running/held state (see lockState below). The schedule endpoints live
	// here too, for proximity to the sync they configure, but are NOT
	// lock-gated: changing whether/when a sync runs automatically does not
	// bypass the mass-deletion guard the way raising the threshold would.
	settingsRepo organization.Repository
	providerName string
	// scheduleLoc is the IANA location every scheduled occurrence is computed
	// in (ORG_SYNC_SCHEDULE_TZ in main.go, default UTC). Used only when an
	// admin enables or re-enables the schedule -- the scheduler itself
	// receives the same value directly, not through this handler.
	scheduleLoc *time.Location
}

// NewOrganizationProviderHandler creates the handler. scheduleLoc must not be
// nil -- pass time.UTC explicitly where no other zone applies.
func NewOrganizationProviderHandler(
	syncService *services.OrganizationSyncService,
	settingsRepo organization.Repository,
	scheduleLoc *time.Location,
) *OrganizationProviderHandler {
	return &OrganizationProviderHandler{
		syncService:  syncService,
		settingsRepo: settingsRepo,
		providerName: "data-provider",
		scheduleLoc:  scheduleLoc,
	}
}

// lockState reports the current threshold lock, tolerating a nil sync service
// (which only happens in tests that exercise the settings endpoints alone).
//
// An error here MUST be surfaced by the caller as a failure -- never treated
// as "not locked". A claim read error producing a false "editable" response
// would reopen exactly the cross-replica bypass the durable hold flag exists
// to close.
func (h *OrganizationProviderHandler) lockState(ctx context.Context) (services.SyncLockState, error) {
	if h.syncService == nil {
		return services.SyncLockState{}, nil
	}
	return h.syncService.LockState(ctx)
}

// thresholdResponse builds the settings body, including why the threshold is
// frozen and at what value, so a tab that has just loaded knows to disable its
// controls without a second request.
func (h *OrganizationProviderHandler) thresholdResponse(
	state services.SyncLockState,
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

	state, err := h.lockState(c.Request.Context())
	if err != nil {
		// Never a false "editable": a claim read error is reported as a
		// failure, not silently treated as unlocked.
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to determine lock state",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, h.thresholdResponse(state, saved, value, source))
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
	state, err := h.lockState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to determine lock state",
			Message: err.Error(),
		})
		return
	}
	if state.Locked() {
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
	state, err = h.lockState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to determine lock state",
			Message: err.Error(),
		})
		return
	}
	if state.Locked() {
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

	c.JSON(http.StatusOK, h.thresholdResponse(state, &percent, percent, services.ThresholdSourceAdmin))
}

// DismissMassDeletionHold handles
// DELETE /api/v1/admin/organization-provider/sync/hold.
//
// It resolves a hold the administrator has decided not to override -- because
// the provider data is what needs fixing -- and so unfreezes the threshold. It
// applies nothing and deletes nothing: the held sync stays unapplied.
//
// Clears both the in-memory hold and the durable hold_active flag (via
// ClearHold), so the escape hatch works even on a replica whose in-memory
// hold was never set because another replica ran the held sync.
func (h *OrganizationProviderHandler) DismissMassDeletionHold(c *gin.Context) {
	if h.syncService == nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Organization provider is not configured",
			Message: "There is no active synchronization or hold to dismiss",
		})
		return
	}

	// A hold cannot be dismissed out from under a running sync.
	state, err := h.syncService.LockState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to determine lock state",
			Message: err.Error(),
		})
		return
	}
	if state.Syncing {
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:   "A synchronization is already running",
			Message: "Wait for the running synchronization to finish, then try again.",
			Code:    dto.CodeThresholdLocked,
		})
		return
	}

	if err := h.syncService.ClearHold(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to clear the mass-deletion hold",
			Message: err.Error(),
		})
		return
	}

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

	// Re-fetched, not reused: the state captured above predates ClearHold and
	// would still report Held -- the response must reflect the just-cleared
	// state, not the one that justified clearing it.
	freshState, err := h.lockState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to determine lock state",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, h.thresholdResponse(freshState, saved, value, source))
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

// GetOrgSyncSchedule handles
// GET /api/v1/admin/settings/organization-provider/schedule.
func (h *OrganizationProviderHandler) GetOrgSyncSchedule(c *gin.Context) {
	schedule, err := h.settingsRepo.GetOrgSyncSchedule(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch organization sync schedule",
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, orgSyncScheduleResponse(schedule))
}

// UpdateOrgSyncSchedule handles
// PUT /api/v1/admin/settings/organization-provider/schedule.
//
// Enabling initializes or recomputes nextRunAt from now -- it never advances
// an already-scheduled occurrence, which is the scheduler's atomic claim's
// job alone (see application/scheduler). Disabling preserves the saved
// frequency (so re-enabling remembers the admin's last choice, matching
// Weekly-as-default applying only on the very first enable) but clears
// nextRunAt, so a disabled schedule never shows a stale next-run date and
// re-enabling always recomputes fresh.
func (h *OrganizationProviderHandler) UpdateOrgSyncSchedule(c *gin.Context) {
	var req dto.UpdateOrgSyncScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
		return
	}
	if req.Enabled == nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "enabled is required",
			Message: "Provide enabled as true or false",
		})
		return
	}

	current, err := h.settingsRepo.GetOrgSyncSchedule(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch organization sync schedule",
			Message: err.Error(),
		})
		return
	}

	if !*req.Enabled {
		if err := h.settingsRepo.UpdateOrgSyncSchedule(c.Request.Context(), false, current.Frequency, nil); err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "Failed to save organization sync schedule",
				Message: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, orgSyncScheduleResponse(&organization.OrgSyncSchedule{Enabled: false, Frequency: current.Frequency}))
		return
	}

	// Weekly-as-default applies only on the very first enable (no frequency
	// saved yet); a later re-enable with no frequency in the request reuses
	// whatever was preserved from the last disable.
	frequency := current.Frequency
	if req.Frequency != nil && *req.Frequency != "" {
		frequency = *req.Frequency
	}
	if frequency == "" {
		frequency = organization.OrgSyncFrequencyWeekly
	}
	if !scheduler.ValidFrequency(frequency) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "frequency must be one of: daily, weekly, monthly",
			Message: "Provide frequency as one of: daily, weekly, monthly",
		})
		return
	}

	nextRunAt := scheduler.NextOccurrence(frequency, time.Now().UTC(), h.scheduleLoc)
	if err := h.settingsRepo.UpdateOrgSyncSchedule(c.Request.Context(), true, frequency, &nextRunAt); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to save organization sync schedule",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, orgSyncScheduleResponse(&organization.OrgSyncSchedule{Enabled: true, Frequency: frequency, NextRunAt: &nextRunAt}))
}

// orgSyncScheduleResponse builds the schedule settings body, including the
// fixed set of choices the UI presents -- never raw cron syntax -- so a tab
// that has just loaded can render its picker without a second request.
func orgSyncScheduleResponse(schedule *organization.OrgSyncSchedule) dto.OrgSyncScheduleDTO {
	resp := dto.OrgSyncScheduleDTO{
		Enabled:   schedule.Enabled,
		Frequency: schedule.Frequency,
		AvailableFrequencies: []string{
			organization.OrgSyncFrequencyDaily,
			organization.OrgSyncFrequencyWeekly,
			organization.OrgSyncFrequencyMonthly,
		},
		DefaultFrequency: organization.OrgSyncFrequencyWeekly,
	}
	if schedule.NextRunAt != nil {
		formatted := schedule.NextRunAt.UTC().Format(time.RFC3339)
		resp.NextRunAt = &formatted
	}
	return resp
}

// GetOrgSyncLastRun handles
// GET /api/v1/admin/organization-provider/sync/last-run.
//
// Reports the persisted result of the most recent attempt that actually ran,
// separately from the most recent skip, so a skip can never mask the last
// actionable result -- see orgprovider.OrgSyncLastRun's doc.
func (h *OrganizationProviderHandler) GetOrgSyncLastRun(c *gin.Context) {
	if h.syncService == nil {
		c.JSON(http.StatusOK, dto.OrgSyncLastRunDTO{})
		return
	}
	last, err := h.syncService.GetLastRun(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch the last organization sync result",
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, orgSyncLastRunResponse(last))
}

func orgSyncLastRunResponse(last *orgprovider.OrgSyncLastRun) dto.OrgSyncLastRunDTO {
	var resp dto.OrgSyncLastRunDTO
	if last == nil {
		return resp
	}
	if a := last.LastAttempt; a != nil {
		resp.LastAttempt = &dto.OrgSyncAttemptDTO{
			RunID:            a.RunID,
			Trigger:          a.Trigger,
			Status:           a.Status,
			StartedAt:        a.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:       a.FinishedAt.UTC().Format(time.RFC3339),
			ThresholdPercent: a.ThresholdPercent,
			UsersExisting:    a.UsersExisting,
			UsersDeleting:    a.UsersDeleting,
			UsersPercent:     a.UsersPercent,
			TeamsExisting:    a.TeamsExisting,
			TeamsDeleting:    a.TeamsDeleting,
			TeamsPercent:     a.TeamsPercent,
			WritesStatus:     a.WritesStatus,
			OverrideUsed:     a.OverrideUsed,
			UsersSynced:      a.UsersSynced,
			TeamsSynced:      a.TeamsSynced,
			Message:          a.Message,
		}
	}
	if sk := last.LastSkip; sk != nil {
		resp.LastSkip = &dto.OrgSyncSkipDTO{
			RunID:  sk.RunID,
			Reason: sk.Reason,
			At:     sk.At.UTC().Format(time.RFC3339),
		}
	}
	return resp
}

// SyncRunIDHeader carries the run ID of a manual sync on every response to
// POST .../sync, success or failure, so an admin can quote it and an operator
// can find that sync's log lines.
const SyncRunIDHeader = "X-Sync-Run-Id"

const (
	defaultSyncHistoryLimit = 50
	maxSyncHistoryLimit     = 200
)

// GetOrgSyncHistory handles
// GET /api/v1/admin/organization-provider/sync/history?limit=N.
//
// Returns the most recent sync outcomes -- manual and scheduled, success,
// blocked, failed and skipped -- newest first. limit defaults to 50 and is
// capped at 200.
func (h *OrganizationProviderHandler) GetOrgSyncHistory(c *gin.Context) {
	limit := defaultSyncHistoryLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:   "Invalid limit",
				Message: "limit must be a positive integer",
			})
			return
		}
		limit = min(n, maxSyncHistoryLimit)
	}

	if h.syncService == nil {
		c.JSON(http.StatusOK, dto.OrgSyncHistoryDTO{Entries: []dto.OrgSyncHistoryEntryDTO{}})
		return
	}
	entries, err := h.syncService.GetHistory(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Failed to fetch the organization sync history",
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, orgSyncHistoryResponse(entries))
}

func orgSyncHistoryResponse(entries []orgprovider.OrgSyncHistoryEntry) dto.OrgSyncHistoryDTO {
	resp := dto.OrgSyncHistoryDTO{Entries: make([]dto.OrgSyncHistoryEntryDTO, 0, len(entries))}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, dto.OrgSyncHistoryEntryDTO{
			RunID:            e.RunID,
			Trigger:          e.Trigger,
			Status:           e.Status,
			StartedAt:        e.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:       e.FinishedAt.UTC().Format(time.RFC3339),
			InstanceID:       e.InstanceID,
			ThresholdPercent: e.ThresholdPercent,
			UsersExisting:    e.UsersExisting,
			UsersDeleting:    e.UsersDeleting,
			UsersPercent:     e.UsersPercent,
			TeamsExisting:    e.TeamsExisting,
			TeamsDeleting:    e.TeamsDeleting,
			TeamsPercent:     e.TeamsPercent,
			WritesStatus:     e.WritesStatus,
			OverrideUsed:     e.OverrideUsed,
			UsersSynced:      e.UsersSynced,
			TeamsSynced:      e.TeamsSynced,
			SkipReason:       e.SkipReason,
			Message:          e.Message,
		})
	}
	return resp
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

	// Generated here, before the sync starts, so the ID reaches the caller on
	// every outcome -- including errors -- as a header, and is the same ID the
	// sync stamps on its log lines and stores in the database.
	runID := uuid.NewString()
	c.Header(SyncRunIDHeader, runID)

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
		RunID:                 runID,
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
		// The trigger-aware message names what is actually running when
		// known ("an automatic scheduled sync"), degrading to the vaguer "a
		// synchronization" when it cannot be determined -- never guessed.
		message := "Another synchronization is already in progress. Wait for it to finish, then try again."
		var inProgress *services.SyncInProgressError
		if errors.As(err, &inProgress) {
			message = "Cannot start a manual sync because " + inProgress.Error() + ". Wait for it to finish, then try again."
		}
		return http.StatusConflict, dto.ErrorResponse{
			Error:   "A synchronization is already running",
			Message: message,
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

	case errors.Is(err, orgprovider.ErrSyncCommitAmbiguous):
		// Distinct from the default branch deliberately: this is the one
		// outcome the sync cannot confirm either way (the write may have
		// committed on the server with the acknowledgment lost), so the
		// message says so explicitly rather than reading as an ordinary
		// failure an admin might assume changed nothing.
		return http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "Synchronization outcome could not be confirmed",
			Message: "The database did not confirm whether this sync's changes were saved. Verify the current user and team counts directly before re-running the sync.",
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
