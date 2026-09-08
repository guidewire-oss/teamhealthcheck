package services

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// EnvMaxDeletePercent names the environment variable configuring the
// mass-deletion guard. See defaultMaxDeletePercent for the fallback.
const EnvMaxDeletePercent = "ORG_SYNC_MAX_DELETE_PERCENT"

// defaultMaxDeletePercent is deliberately conservative: a sync that would
// remove more than a fifth of the currently-synced, non-protected users or
// teams in one run is more likely a bad/incomplete snapshot than a real
// mass-departure event, and should be held for human review rather than
// applied automatically.
const defaultMaxDeletePercent = 20.0

// Errors returned by OrganizationSyncService. Handlers map these to status codes.
var (
	// ErrSyncInProgress means another synchronization is already running.
	ErrSyncInProgress = errors.New("a synchronization is already in progress")
	// ErrProviderNotConfigured means the data provider client is not configured.
	ErrProviderNotConfigured = errors.New("organization provider is not configured")
	// ErrInvalidSnapshot means the provider returned data that breaches the contract.
	ErrInvalidSnapshot = errors.New("provider snapshot failed contract validation")
)

// SnapshotFetcher fetches a complete organization snapshot from an external
// provider. The sync service depends on this interface, not a concrete
// client, so a second provider can be added without touching orchestration.
// The fetcher owns its own credentials (read from its own environment
// configuration); it is never handed a token by this service.
type SnapshotFetcher interface {
	FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error)
}

// SyncResult is the outcome of one synchronization run, returned to the API
// caller. It never carries a token, header, or the raw provider payload.
type SyncResult struct {
	Status string `json:"status"`

	TeamsSynced        int `json:"teamsSynced"`
	UsersSynced        int `json:"usersSynced"`
	MembershipsSynced  int `json:"membershipsSynced"`
	MembershipsRemoved int `json:"membershipsRemoved"`

	HealthChecksDisabled int `json:"healthChecksDisabled"`
	HealthChecksEnabled  int `json:"healthChecksEnabled"`

	UsersDeleted       int `json:"usersDeleted"`
	TeamsDeleted       int `json:"teamsDeleted"`
	ActionItemsDeleted int `json:"actionItemsDeleted"`

	UsersSkipped         int                       `json:"usersSkipped"`
	SkippedUsers         []orgprovider.SkippedUser `json:"skippedUsers,omitempty"`
	ManagerLinksCleared  int                       `json:"managerLinksCleared"`
	TeamLeadsCleared     int                       `json:"teamLeadsCleared"`
	MembershipsDiscarded int                       `json:"membershipsDiscarded"`

	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

// OrganizationSyncService pulls an organization snapshot from the configured
// provider and applies it to THC.
type OrganizationSyncService struct {
	repo     orgprovider.Repository
	fetcher  SnapshotFetcher
	userRepo user.Repository
	teamRepo team.Repository

	// running admits one sync at a time. A second concurrent request is
	// rejected rather than queued: two runs applying overlapping snapshots
	// would interleave their transactions unpredictably.
	running atomic.Bool
}

// NewOrganizationSyncService creates the service. A nil fetcher is tolerated
// at construction (mirrors the "disabled until configured" convention used
// elsewhere); Sync reports the misconfiguration when actually invoked.
func NewOrganizationSyncService(
	repo orgprovider.Repository,
	fetcher SnapshotFetcher,
	userRepo user.Repository,
	teamRepo team.Repository,
) *OrganizationSyncService {
	return &OrganizationSyncService{
		repo:     repo,
		fetcher:  fetcher,
		userRepo: userRepo,
		teamRepo: teamRepo,
	}
}

// Configured reports whether a sync could run at all.
func (s *OrganizationSyncService) Configured() bool {
	return s.fetcher != nil
}

// Sync fetches, validates and applies a snapshot.
//
// Returns ErrSyncInProgress when another run holds the lock. All persistence
// happens in a single transaction, so a failure at any point -- including the
// mass-deletion guard tripping -- leaves THC untouched.
func (s *OrganizationSyncService) Sync(ctx context.Context) (*SyncResult, error) {
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrSyncInProgress
	}
	defer s.running.Store(false)

	log := logger.Get()
	startedAt := time.Now().UTC()

	if s.fetcher == nil {
		return nil, ErrProviderNotConfigured
	}

	snapshot, err := s.fetcher.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	knownLevels, err := s.repo.KnownHierarchyLevelIDs(ctx)
	if err != nil {
		return nil, err
	}

	filtered := FilterSnapshot(snapshot, knownLevels)

	if err := filtered.Snapshot.ValidationError(); err != nil {
		// Validation errors name records, not credentials, so they are safe to
		// return to an admin and are the only way to diagnose a bad snapshot.
		return nil, errors.Join(ErrInvalidSnapshot, err)
	}

	applied, err := s.repo.ApplySnapshot(ctx, orgprovider.ApplyInput{
		Snapshot:                 filtered.Snapshot,
		PreservedMemberUserIDs:   filtered.PreservedMemberUserIDs,
		PreserveReportsToUserIDs: filtered.PreserveReportsToUserIDs,
		MaxDeletePercent:         maxDeletePercent(),
	})
	if err != nil {
		return nil, err
	}

	// The supervisor chain is a denormalized cache derived from reports_to.
	// Rebuilding it is best-effort and deliberately outside the transaction:
	// the sync has already committed, and a stale cache is recoverable whereas
	// a rolled-back org import is not. A failure here is logged but does not
	// change the sync's reported status -- it is a real (if narrow) gap
	// between "the sync completed" and "every derived structure is
	// consistent," called out explicitly rather than silently claimed away.
	supervisorChainErr := s.rederiveSupervisorChains(ctx, filtered.Snapshot.Teams)

	result := &SyncResult{
		Status:               "completed",
		TeamsSynced:          applied.TeamsSynced,
		UsersSynced:          applied.UsersSynced,
		MembershipsSynced:    applied.MembershipsSynced,
		MembershipsRemoved:   applied.MembershipsRemoved,
		HealthChecksDisabled: applied.HealthChecksDisabled,
		HealthChecksEnabled:  applied.HealthChecksEnabled,
		UsersDeleted:         applied.UsersDeleted,
		TeamsDeleted:         applied.TeamsDeleted,
		ActionItemsDeleted:   applied.ActionItemsDeleted,
		UsersSkipped:         len(filtered.SkippedUsers),
		SkippedUsers:         filtered.SkippedUsers,
		ManagerLinksCleared:  filtered.ClearedManagers,
		TeamLeadsCleared:     filtered.ClearedTeamLeads,
		MembershipsDiscarded: filtered.DroppedMemberships + filtered.DuplicateMemberships,
		StartedAt:            startedAt,
		CompletedAt:          time.Now().UTC(),
	}
	if supervisorChainErr != nil {
		result.Status = "completed_with_warnings"
	}

	log.WithFields(map[string]interface{}{
		"usersSynced":       result.UsersSynced,
		"teamsSynced":       result.TeamsSynced,
		"membershipsSynced": result.MembershipsSynced,
		"usersDeleted":      result.UsersDeleted,
		"teamsDeleted":      result.TeamsDeleted,
		"usersSkipped":      result.UsersSkipped,
	}).Info("organization provider sync completed")

	return result, nil
}

// maxDeletePercent reads the configurable mass-deletion threshold, falling
// back to defaultMaxDeletePercent when unset or invalid.
func maxDeletePercent() float64 {
	raw := os.Getenv(EnvMaxDeletePercent)
	if raw == "" {
		return defaultMaxDeletePercent
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return defaultMaxDeletePercent
	}
	return value
}

// rederiveSupervisorChains refreshes team_supervisors for the synced teams,
// mirroring the derivation the admin team handler performs after a lead
// changes. Returns a non-nil error if any team's chain failed to rebuild, so
// the caller can surface that the post-commit step was incomplete.
func (s *OrganizationSyncService) rederiveSupervisorChains(ctx context.Context, teams []orgsnapshot.Team) error {
	if s.userRepo == nil || s.teamRepo == nil {
		return nil
	}

	log := logger.Get()
	var firstErr error

	for _, t := range teams {
		stored, err := s.teamRepo.FindByID(ctx, t.ID)
		if err != nil || stored == nil || stored.TeamLeadID == nil {
			continue
		}

		supervisors, err := s.userRepo.FindSupervisorChainUp(ctx, *stored.TeamLeadID)
		if err != nil {
			log.Warn("failed to derive supervisor chain for team " + t.ID + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		chain := make([]*team.SupervisorLink, len(supervisors))
		for i, sup := range supervisors {
			chain[i] = &team.SupervisorLink{UserID: sup.ID, LevelID: sup.HierarchyLevelID}
		}

		if err := s.teamRepo.UpdateSupervisorChain(ctx, t.ID, chain); err != nil {
			log.Warn("failed to save derived supervisor chain for team " + t.ID + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}
