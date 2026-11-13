package v1

import (
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

const (
	defaultSyncHistoryPageSize = 20
	maxSyncHistoryPageSize     = 100
	// maxHistoryErrorLen caps the error text shown to an administrator.
	maxHistoryErrorLen = 300
)

// ListSyncRunHistory handles GET /api/v1/admin/organization-provider/sync/runs.
//
// Query: page (default 1), pageSize (default 20, max 100), status, trigger,
// snapshotType. An unknown filter value is a 400 rather than silently ignored,
// so a typo cannot masquerade as "no failures".
func (h *OrganizationProviderHandler) ListSyncRunHistory(c *gin.Context) {
	if h.syncService == nil || !h.syncService.RunsEnabled() {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Sync runs are not available"})
		return
	}

	filter := orgsyncrun.HistoryFilter{}
	if v := c.Query("status"); v != "" {
		if !orgsyncrun.ValidStatus(v) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status filter", Message: "status must be one of PENDING, RUNNING, SUCCESS, FAILED"})
			return
		}
		filter.Status = orgsyncrun.Status(v)
	}
	if v := c.Query("trigger"); v != "" {
		if !orgsyncrun.ValidTrigger(v) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid trigger filter", Message: "trigger must be one of MANUAL, SCHEDULED"})
			return
		}
		filter.Trigger = orgsyncrun.Trigger(v)
	}
	if v := c.Query("snapshotType"); v != "" {
		if !orgsyncrun.ValidSnapshotType(v) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid snapshotType filter", Message: "snapshotType must be one of COMPLETE, PARTIAL"})
			return
		}
		filter.SnapshotType = orgprovider.SnapshotType(v)
	}

	pageSize := min(parsePositiveIntParam(c, "pageSize", defaultSyncHistoryPageSize), maxSyncHistoryPageSize)
	// page is any positive integer from the caller. Clamp it before multiplying,
	// or (page-1)*pageSize can overflow int, go negative and make PostgreSQL
	// reject the OFFSET with a 500. A page this far out is past the last page
	// anyway, so it returns an empty list like any other out-of-range page.
	page := min(parsePositiveIntParam(c, "page", 1), math.MaxInt32/pageSize)
	filter.Limit = pageSize
	filter.Offset = (page - 1) * pageSize

	runs, total, err := h.syncService.ListRuns(c.Request.Context(), filter)
	switch {
	case errors.Is(err, services.ErrRunHistoryUnavailable):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Sync run history is not available"})
		return
	case err != nil:
		logger.Get().WithError(err).Error("failed to list sync run history")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to load sync history"})
		return
	}

	items := make([]dto.SyncRunHistoryItemDTO, len(runs))
	for i := range runs {
		items[i] = syncRunHistoryItem(&runs[i])
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	c.JSON(http.StatusOK, dto.SyncRunHistoryResponse{
		Runs: items,
		Pagination: dto.PaginationDTO{
			Page: page, PageSize: pageSize, TotalItems: total, TotalPages: totalPages,
			HasNextPage: page < totalPages, HasPreviousPage: page > 1,
		},
	})
}

// syncRunHistoryItem maps a persisted run onto the admin history shape.
func syncRunHistoryItem(r *orgsyncrun.Run) dto.SyncRunHistoryItemDTO {
	d := dto.SyncRunHistoryItemDTO{
		RunID: r.ID, SnapshotType: string(r.SnapshotType), Trigger: string(r.Trigger),
		Status: string(r.Status), Stage: string(r.Stage), InitiatedBy: r.InitiatedBy,
		CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
		SourceGeneratedAt: r.SourceGeneratedAt,
	}
	stale := r.ErrorCode != nil && *r.ErrorCode == orgsyncrun.CodeStaleRun
	if r.StartedAt != nil && r.CompletedAt != nil && !stale && !r.CompletedAt.Before(*r.StartedAt) {
		secs := int64(r.CompletedAt.Sub(*r.StartedAt) / time.Second)
		d.DurationSeconds = &secs
	}
	if r.Status == orgsyncrun.StatusSuccess {
		d.InsertedUsers, d.UpdatedUsers, d.UnchangedUsers = &r.InsertedUsers, &r.UpdatedUsers, r.UnchangedUsers
		d.InsertedTeams, d.UpdatedTeams, d.UnchangedTeams = &r.InsertedTeams, &r.UpdatedTeams, r.UnchangedTeams
		d.MembershipChanges = &r.MembershipChanges
	}
	if r.Status == orgsyncrun.StatusFailed {
		if r.FailedStage != nil {
			fs := string(*r.FailedStage)
			d.FailedStage = &fs
		}
		d.ErrorCode = r.ErrorCode
		if r.ErrorMessage != nil {
			msg := sanitizeRunError(*r.ErrorMessage)
			d.ErrorMessage = &msg
		}
	}
	return d
}

var (
	// credential-looking "key: value" / "key=value" pairs and bearer tokens.
	//
	// The key has no leading \b on purpose: \b never matches inside an
	// identifier, so it would miss db_password=, ACCESS_TOKEN= and clientSecret:.
	// The key must still be followed by an optional closing quote (plain, or
	// backslash-escaped inside an embedded JSON string) and then a separator, so
	// a word that merely starts with a key name (tokenizer, secretary) is left
	// alone. This covers "password": "x" and 'password': 'x' as well as password=x.
	// An unquoted value runs to the next whitespace, which can redact more than
	// the secret; that is the safe direction for this filter.
	secretPairRe = regexp.MustCompile(`(?i)(authorization|api[_-]?key|apikey|token|secret|password|passwd|pwd|credentials?)((?:\\?["'])?\s*[:=]\s*)("[^"]*"|'[^']*'|\S+)`)
	bearerRe     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	urlUserRe    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@]+(:[^/\s@]*)?@`)
	urlQueryRe   = regexp.MustCompile(`(\bhttps?://[^\s?#]+)\?[^\s]*`)
)

// sanitizeRunError is defense in depth. Failure messages are written as
// "already safe to show an administrator", but the column is free text, so the
// history view still redacts credential-looking content, strips URL userinfo and
// query strings, collapses whitespace and caps the length.
func sanitizeRunError(msg string) string {
	msg = bearerRe.ReplaceAllString(msg, "$1 [redacted]")
	msg = secretPairRe.ReplaceAllString(msg, "$1$2[redacted]")
	msg = urlUserRe.ReplaceAllString(msg, "${1}[redacted]@")
	msg = urlQueryRe.ReplaceAllString(msg, "$1?[redacted]")
	msg = strings.Join(strings.Fields(msg), " ")
	if utf8.RuneCountInString(msg) > maxHistoryErrorLen {
		msg = string([]rune(msg)[:maxHistoryErrorLen]) + "…"
	}
	return msg
}
