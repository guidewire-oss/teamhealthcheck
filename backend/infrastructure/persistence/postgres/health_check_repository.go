package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres/sqlcgen"
)

// uniqueViolationConstraints maps the Postgres unique-index names enforcing "one submission
// per (scope, survey type, period)" to the survey type they guard, so a 23505
// (unique violation) error from Save() can be translated into a friendly
// healthcheck.DuplicateSubmissionError.
var uniqueViolationConstraints = map[string]string{
	"uniq_individual_period_submission":    healthcheck.SurveyTypeIndividual,
	"uniq_post_workshop_period_submission": healthcheck.SurveyTypePostWorkshop,
}

// HealthCheckRepository implements the healthcheck.Repository interface
type HealthCheckRepository struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewHealthCheckRepository creates a new repository instance
func NewHealthCheckRepository(db *sql.DB) healthcheck.Repository {
	return &HealthCheckRepository{db: db, queries: sqlcgen.New(db)}
}

// Save persists a health check session and its responses (atomic operation)
func (r *HealthCheckRepository) Save(ctx context.Context, session *healthcheck.HealthCheckSession) error {
	// Default survey type to "individual" if empty
	surveyType := session.SurveyType
	if surveyType == "" {
		surveyType = healthcheck.SurveyTypeIndividual
	}

	// Begin transaction for atomic save
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := r.queries.WithTx(tx)

	// Enforce enrollment in the same transaction as the write. The command checks the flag
	// first for a fast answer, but that read is not atomic with this insert: a PodIQ sync could
	// commit health_check_enabled = false in between. FOR SHARE locks the team row until this
	// transaction ends, and the sync's UPDATE needs a conflicting lock, so either the sync
	// commits first (the flag is false here and the save is rejected) or it waits for this save.
	// An unknown team is left to the foreign key.
	enabled, err := qtx.LockTeamHealthCheckEnabled(ctx, session.TeamID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return &healthcheck.ParticipationLookupError{TeamID: session.TeamID, Err: err}
	case !enabled:
		return &healthcheck.ParticipationDisabledError{TeamID: session.TeamID}
	}

	// Insert or update session
	err = qtx.UpsertHealthCheckSession(ctx, sqlcgen.UpsertHealthCheckSessionParams{
		ID:               session.ID,
		TeamID:           session.TeamID,
		UserID:           session.UserID,
		Date:             session.Date,
		AssessmentPeriod: sql.NullString{String: session.AssessmentPeriod, Valid: true},
		SurveyType:       surveyType,
		Completed:        sql.NullBool{Bool: session.Completed, Valid: true},
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if constraintSurveyType, known := uniqueViolationConstraints[pqErr.Constraint]; known {
				// The failed insert aborted this transaction, and the lookup below runs on
				// another pooled connection. Give this one back first: holding it while
				// waiting for a second would let concurrent duplicate submissions exhaust a
				// small pool and time out instead of getting a clean 409.
				_ = tx.Rollback()
				submitted, serr := r.FindSubmittedPeriods(ctx, healthcheck.LatestSubmissionQuery{
					SurveyType: constraintSurveyType,
					TeamID:     session.TeamID,
					UserID:     session.UserID,
				})
				if serr == nil {
					return healthcheck.NewDuplicateSubmissionError(constraintSurveyType, session.AssessmentPeriod, submitted)
				}
				return fmt.Errorf("submission rejected: %s for %s already submitted", constraintSurveyType, session.AssessmentPeriod)
			}
		}
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Delete existing responses (for updates)
	if err := qtx.DeleteHealthCheckResponsesBySessionID(ctx, session.ID); err != nil {
		return fmt.Errorf("failed to delete existing responses: %w", err)
	}

	// Insert responses
	for _, response := range session.Responses {
		err = qtx.InsertHealthCheckResponse(ctx, sqlcgen.InsertHealthCheckResponseParams{
			SessionID:   session.ID,
			DimensionID: response.DimensionID,
			Score:       int16(response.Score),
			Trend:       response.Trend,
			Comment:     sql.NullString{String: response.Comment, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to save response: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FindByID retrieves a session by ID
func (r *HealthCheckRepository) FindByID(ctx context.Context, id string) (*healthcheck.HealthCheckSession, error) {
	rows, err := r.queries.FindSessionRowsByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	sessions := buildSessions(sessionScanRowsFromByID(rows))
	if len(sessions) == 0 {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	return sessions[0], nil
}

// FindByTeamID retrieves all sessions for a team
func (r *HealthCheckRepository) FindByTeamID(ctx context.Context, teamID string) ([]*healthcheck.HealthCheckSession, error) {
	rows, err := r.queries.FindSessionRowsByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return buildSessions(sessionScanRowsFromByTeamID(rows)), nil
}

// FindByUserID retrieves all sessions for a user
func (r *HealthCheckRepository) FindByUserID(ctx context.Context, userID string) ([]*healthcheck.HealthCheckSession, error) {
	rows, err := r.queries.FindSessionRowsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return buildSessions(sessionScanRowsFromByUserID(rows)), nil
}

// FindByAssessmentPeriod retrieves all sessions for an assessment period
func (r *HealthCheckRepository) FindByAssessmentPeriod(ctx context.Context, period string) ([]*healthcheck.HealthCheckSession, error) {
	rows, err := r.queries.FindSessionRowsByAssessmentPeriod(ctx, sql.NullString{String: period, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return buildSessions(sessionScanRowsFromByAssessmentPeriod(rows)), nil
}

// FindTeamHealthByManager retrieves aggregated health data for teams under a manager.
// Prefers post-workshop sessions when available for a team+period, otherwise falls back to individual sessions.
func (r *HealthCheckRepository) FindTeamHealthByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.TeamHealthSummary, error) {
	var teamRows []teamHealthScanRow

	// Use effective_sessions CTE to prefer post_workshop data when available
	if assessmentPeriod != "" {
		rows, err := r.queries.FindTeamHealthByManagerWithPeriod(ctx, sqlcgen.FindTeamHealthByManagerWithPeriodParams{
			ManagerID:        managerID,
			AssessmentPeriod: sql.NullString{String: assessmentPeriod, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query team health by manager: %w", err)
		}
		teamRows = make([]teamHealthScanRow, len(rows))
		for i, row := range rows {
			teamRows[i] = teamHealthScanRow{
				TeamID: row.TeamID, TeamName: row.TeamName, SubmissionCount: row.SubmissionCount,
				OverallHealth: row.OverallHealth, PostWorkshopStatus: row.PostWorkshopStatus,
				DimensionID: row.DimensionID, AvgScore: row.AvgScore, ResponseCount: row.ResponseCount,
			}
		}
	} else {
		rows, err := r.queries.FindTeamHealthByManagerAllPeriods(ctx, managerID)
		if err != nil {
			return nil, fmt.Errorf("failed to query team health by manager: %w", err)
		}
		teamRows = make([]teamHealthScanRow, len(rows))
		for i, row := range rows {
			teamRows[i] = teamHealthScanRow{
				TeamID: row.TeamID, TeamName: row.TeamName, SubmissionCount: row.SubmissionCount,
				OverallHealth: row.OverallHealth, PostWorkshopStatus: row.PostWorkshopStatus,
				DimensionID: row.DimensionID, AvgScore: row.AvgScore, ResponseCount: row.ResponseCount,
			}
		}
	}

	// Group results by team (order is already by overall_health ASC from query)
	teamsMap := make(map[string]*healthcheck.TeamHealthSummary)
	teamOrder := []string{}

	for _, row := range teamRows {
		team, exists := teamsMap[row.TeamID]
		if !exists {
			health := 0.0
			if row.OverallHealth.Valid {
				health = row.OverallHealth.Float64
			}
			team = &healthcheck.TeamHealthSummary{
				TeamID:             row.TeamID,
				TeamName:           row.TeamName,
				SubmissionCount:    int(row.SubmissionCount),
				OverallHealth:      health,
				Dimensions:         []healthcheck.DimensionSummary{},
				PostWorkshopStatus: row.PostWorkshopStatus,
			}
			teamsMap[row.TeamID] = team
			teamOrder = append(teamOrder, row.TeamID)
		}

		if row.DimensionID.Valid && row.AvgScore.Valid {
			team.Dimensions = append(team.Dimensions, healthcheck.DimensionSummary{
				DimensionID:   row.DimensionID.String,
				AvgScore:      row.AvgScore.Float64,
				ResponseCount: int(row.ResponseCount.Int64),
			})
		}
	}

	teams := make([]healthcheck.TeamHealthSummary, 0, len(teamOrder))
	for _, id := range teamOrder {
		teams = append(teams, *teamsMap[id])
	}

	return teams, nil
}

// FindAggregatedDimensionsByManager retrieves aggregated dimension data across all teams under a manager.
// Prefers post-workshop sessions when available for a team+period, otherwise falls back to individual sessions.
func (r *HealthCheckRepository) FindAggregatedDimensionsByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.DimensionSummary, error) {
	dimensions := []healthcheck.DimensionSummary{}

	if assessmentPeriod != "" {
		rows, err := r.queries.FindAggregatedDimensionsByManagerWithPeriod(ctx, sqlcgen.FindAggregatedDimensionsByManagerWithPeriodParams{
			ManagerID:        managerID,
			AssessmentPeriod: sql.NullString{String: assessmentPeriod, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query aggregated dimensions by manager: %w", err)
		}
		for _, row := range rows {
			dimensions = append(dimensions, healthcheck.DimensionSummary{
				DimensionID:   row.DimensionID,
				AvgScore:      row.AvgScore,
				ResponseCount: int(row.ResponseCount),
			})
		}
	} else {
		rows, err := r.queries.FindAggregatedDimensionsByManagerAllPeriods(ctx, managerID)
		if err != nil {
			return nil, fmt.Errorf("failed to query aggregated dimensions by manager: %w", err)
		}
		for _, row := range rows {
			dimensions = append(dimensions, healthcheck.DimensionSummary{
				DimensionID:   row.DimensionID,
				AvgScore:      row.AvgScore,
				ResponseCount: int(row.ResponseCount),
			})
		}
	}

	return dimensions, nil
}

// GetTeamSubmissionStatus returns submission status for a team in a given assessment period
func (r *HealthCheckRepository) GetTeamSubmissionStatus(ctx context.Context, teamID string, assessmentPeriod string) (*healthcheck.TeamSubmissionStatus, error) {
	row, err := r.queries.GetTeamSubmissionStatus(ctx, sqlcgen.GetTeamSubmissionStatusParams{
		TeamID:           teamID,
		AssessmentPeriod: sql.NullString{String: assessmentPeriod, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query team submission status: %w", err)
	}

	totalMembers := int(row.TotalMembers)
	submittedMembers := int(row.SubmittedMembers)
	allSubmitted := totalMembers > 0 && submittedMembers >= totalMembers

	return &healthcheck.TeamSubmissionStatus{
		TeamID:             teamID,
		AssessmentPeriod:   assessmentPeriod,
		TotalMembers:       totalMembers,
		SubmittedMembers:   submittedMembers,
		AllSubmitted:       allSubmitted,
		PostWorkshopExists: row.PostWorkshopExists,
	}, nil
}

// FindSubmittedPeriods returns the assessment-period labels of every completed submission
// matching the given survey type and scope, or an empty slice if none exist. Scoping:
//   - individual:    matches by UserID only (a different user's submission never conflicts).
//   - post_workshop: matches by TeamID only (one workshop consensus per team).
//
// Historical sessions that never recorded a period are left out: they cannot be matched to a
// period, so they neither block a new submission nor break the scan below.
//
// This is the persisted source of truth healthcheck.CheckPeriodEligibility and
// healthcheck.NextEligiblePeriodAfter are evaluated against -- eligibility is never inferred
// from submission dates.
func (r *HealthCheckRepository) FindSubmittedPeriods(ctx context.Context, query healthcheck.LatestSubmissionQuery) ([]string, error) {
	var rows []sql.NullString
	var err error
	if query.SurveyType == healthcheck.SurveyTypePostWorkshop {
		rows, err = r.queries.FindSubmittedPeriodsByTeam(ctx, sqlcgen.FindSubmittedPeriodsByTeamParams{TeamID: query.TeamID, SurveyType: query.SurveyType})
	} else {
		rows, err = r.queries.FindSubmittedPeriodsByUser(ctx, sqlcgen.FindSubmittedPeriodsByUserParams{UserID: query.UserID, SurveyType: query.SurveyType})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query submitted periods: %w", err)
	}

	var periods []string
	for _, row := range rows {
		if row.Valid {
			periods = append(periods, row.String)
		}
	}
	return periods, nil
}

// Delete removes a session and its responses (cascade handled by DB)
func (r *HealthCheckRepository) Delete(ctx context.Context, id string) error {
	rowsAffected, err := r.queries.DeleteHealthCheckSession(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// FindDistinctAssessmentPeriods returns the unique SELECTABLE assessment
// periods (half-years only) from submitted sessions, most recent
// first. Ordering is done in Go
// (healthcheck.SortAssessmentPeriodsDescending) rather than in SQL: this
// app's period strings come in five different formats (monthly, quarterly,
// half-yearly, yearly, and a legacy half-year format — see
// frontend/lib/assessment-period.ts), and a plain `ORDER BY
// assessment_period DESC` is a lexicographic STRING sort that has no
// concept of month/quarter order across those formats (e.g. "H1" < "Mar" <
// "Q1" alphabetically, unrelated to actual chronological order).
func (r *HealthCheckRepository) FindDistinctAssessmentPeriods(ctx context.Context) ([]string, error) {
	rows, err := r.queries.FindDistinctAssessmentPeriods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query assessment periods: %w", err)
	}

	periods := make([]string, 0, len(rows))
	for _, period := range rows {
		periods = append(periods, period.String)
	}

	// Only half-year periods ("YYYY H1"/"YYYY H2", or the older
	// "YYYY - 1st/2nd Half" label) are selectable;
	// stored sessions with any other value stay in the table but are never
	// offered (see healthcheck.IsSelectableAssessmentPeriod). This is the one
	// source for every period dropdown and for the admin dashboard's period
	// validation, so all dashboards agree.
	return healthcheck.SelectableAssessmentPeriods(periods), nil
}

// Helper types and functions

// sessionScanRow is the common shape shared by every FindSessionRowsBy*Row
// generated type - sqlc generates a distinct struct per query even though
// FindSessionRowsByID/ByTeamID/ByUserID/ByAssessmentPeriod share one SELECT list.
type sessionScanRow struct {
	SessionID        string
	TeamID           string
	UserID           string
	Date             string
	AssessmentPeriod sql.NullString
	SurveyType       string
	Completed        bool
	DimensionID      sql.NullString
	Score            sql.NullInt16
	Trend            sql.NullString
	Comment          sql.NullString
}

func sessionScanRowsFromByID(rows []sqlcgen.FindSessionRowsByIDRow) []sessionScanRow {
	out := make([]sessionScanRow, len(rows))
	for i, row := range rows {
		out[i] = sessionScanRow{
			SessionID: row.ID, TeamID: row.TeamID, UserID: row.UserID, Date: row.Date,
			AssessmentPeriod: row.AssessmentPeriod, SurveyType: row.SurveyType, Completed: row.Completed.Bool, // NULL reads as false
			DimensionID: row.DimensionID, Score: row.Score, Trend: row.Trend, Comment: row.Comment,
		}
	}
	return out
}

func sessionScanRowsFromByTeamID(rows []sqlcgen.FindSessionRowsByTeamIDRow) []sessionScanRow {
	out := make([]sessionScanRow, len(rows))
	for i, row := range rows {
		out[i] = sessionScanRow{
			SessionID: row.ID, TeamID: row.TeamID, UserID: row.UserID, Date: row.Date,
			AssessmentPeriod: row.AssessmentPeriod, SurveyType: row.SurveyType, Completed: row.Completed.Bool, // NULL reads as false
			DimensionID: row.DimensionID, Score: row.Score, Trend: row.Trend, Comment: row.Comment,
		}
	}
	return out
}

func sessionScanRowsFromByUserID(rows []sqlcgen.FindSessionRowsByUserIDRow) []sessionScanRow {
	out := make([]sessionScanRow, len(rows))
	for i, row := range rows {
		out[i] = sessionScanRow{
			SessionID: row.ID, TeamID: row.TeamID, UserID: row.UserID, Date: row.Date,
			AssessmentPeriod: row.AssessmentPeriod, SurveyType: row.SurveyType, Completed: row.Completed.Bool, // NULL reads as false
			DimensionID: row.DimensionID, Score: row.Score, Trend: row.Trend, Comment: row.Comment,
		}
	}
	return out
}

func sessionScanRowsFromByAssessmentPeriod(rows []sqlcgen.FindSessionRowsByAssessmentPeriodRow) []sessionScanRow {
	out := make([]sessionScanRow, len(rows))
	for i, row := range rows {
		out[i] = sessionScanRow{
			SessionID: row.ID, TeamID: row.TeamID, UserID: row.UserID, Date: row.Date,
			AssessmentPeriod: row.AssessmentPeriod, SurveyType: row.SurveyType, Completed: row.Completed.Bool, // NULL reads as false
			DimensionID: row.DimensionID, Score: row.Score, Trend: row.Trend, Comment: row.Comment,
		}
	}
	return out
}

// buildSessions groups flat (session, response) rows produced by a LEFT JOIN
// into HealthCheckSession aggregates with their Responses populated -
// mirrors the original scanSessions helper's grouping logic.
func buildSessions(rows []sessionScanRow) []*healthcheck.HealthCheckSession {
	sessionsMap := make(map[string]*healthcheck.HealthCheckSession)
	sessionOrder := []string{}

	for _, row := range rows {
		session, exists := sessionsMap[row.SessionID]
		if !exists {
			st := row.SurveyType
			if st == "" {
				st = healthcheck.SurveyTypeIndividual
			}
			session = &healthcheck.HealthCheckSession{
				ID:               row.SessionID,
				TeamID:           row.TeamID,
				UserID:           row.UserID,
				Date:             row.Date,
				AssessmentPeriod: row.AssessmentPeriod.String,
				SurveyType:       st,
				Completed:        row.Completed,
				Responses:        []healthcheck.HealthCheckResponse{},
			}
			sessionsMap[row.SessionID] = session
			sessionOrder = append(sessionOrder, row.SessionID)
		}

		if row.DimensionID.Valid {
			session.Responses = append(session.Responses, healthcheck.HealthCheckResponse{
				DimensionID: row.DimensionID.String,
				Score:       int(row.Score.Int16),
				Trend:       row.Trend.String,
				Comment:     row.Comment.String,
			})
		}
	}

	sessions := make([]*healthcheck.HealthCheckSession, 0, len(sessionOrder))
	for _, id := range sessionOrder {
		sessions = append(sessions, sessionsMap[id])
	}

	return sessions
}

// teamHealthScanRow is the common shape shared by both
// FindTeamHealthByManagerWithPeriodRow and FindTeamHealthByManagerAllPeriodsRow.
type teamHealthScanRow struct {
	TeamID             string
	TeamName           string
	SubmissionCount    int64
	OverallHealth      sql.NullFloat64
	PostWorkshopStatus string
	DimensionID        sql.NullString
	AvgScore           sql.NullFloat64
	ResponseCount      sql.NullInt64
}

// IsTeamHealthCheckEnabled implements healthcheck.ParticipationChecker.
func (r *HealthCheckRepository) IsTeamHealthCheckEnabled(ctx context.Context, teamID string) (bool, bool, error) {
	enabled, err := r.queries.GetTeamHealthCheckEnabled(ctx, teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("failed to read team participation: %w", err)
	}
	return enabled, true, nil
}
