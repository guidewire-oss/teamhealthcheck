// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
)

// HealthCheckRepository implements the healthcheck.Repository interface
type HealthCheckRepository struct {
	db *sql.DB
}

// NewHealthCheckRepository creates a new repository instance
func NewHealthCheckRepository(db *sql.DB) healthcheck.Repository {
	return &HealthCheckRepository{db: db}
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

	// Insert or update session
	_, err = tx.ExecContext(ctx, `
		INSERT INTO health_check_sessions (
			id, team_id, user_id, date, assessment_period, survey_type, completed, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			team_id = EXCLUDED.team_id,
			user_id = EXCLUDED.user_id,
			date = EXCLUDED.date,
			assessment_period = EXCLUDED.assessment_period,
			survey_type = EXCLUDED.survey_type,
			completed = EXCLUDED.completed,
			updated_at = CURRENT_TIMESTAMP
	`, session.ID, session.TeamID, session.UserID, session.Date, session.AssessmentPeriod, surveyType, session.Completed)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Delete existing responses (for updates)
	_, err = tx.ExecContext(ctx, "DELETE FROM health_check_responses WHERE session_id = $1", session.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing responses: %w", err)
	}

	// Insert responses
	for _, response := range session.Responses {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO health_check_responses (
				session_id, dimension_id, score, trend, comment
			) VALUES ($1, $2, $3, $4, $5)
		`, session.ID, response.DimensionID, response.Score, response.Trend, response.Comment)

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
	sessions, err := r.scanSessions(ctx, `
		SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
		       r.dimension_id, r.score, r.trend, r.comment
		FROM health_check_sessions s
		LEFT JOIN health_check_responses r ON s.id = r.session_id
		WHERE s.id = $1
		ORDER BY r.dimension_id
	`, id)

	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	return sessions[0], nil
}

// FindByTeamID retrieves all sessions for a team
func (r *HealthCheckRepository) FindByTeamID(ctx context.Context, teamID string) ([]*healthcheck.HealthCheckSession, error) {
	return r.scanSessions(ctx, `
		SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
		       r.dimension_id, r.score, r.trend, r.comment
		FROM health_check_sessions s
		LEFT JOIN health_check_responses r ON s.id = r.session_id
		WHERE s.team_id = $1
		ORDER BY s.date DESC, r.dimension_id
	`, teamID)
}

// FindByUserID retrieves all sessions for a user
func (r *HealthCheckRepository) FindByUserID(ctx context.Context, userID string) ([]*healthcheck.HealthCheckSession, error) {
	return r.scanSessions(ctx, `
		SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
		       r.dimension_id, r.score, r.trend, r.comment
		FROM health_check_sessions s
		LEFT JOIN health_check_responses r ON s.id = r.session_id
		WHERE s.user_id = $1
		ORDER BY s.date DESC, r.dimension_id
	`, userID)
}

// FindByAssessmentPeriod retrieves all sessions for an assessment period
func (r *HealthCheckRepository) FindByAssessmentPeriod(ctx context.Context, period string) ([]*healthcheck.HealthCheckSession, error) {
	return r.scanSessions(ctx, `
		SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
		       r.dimension_id, r.score, r.trend, r.comment
		FROM health_check_sessions s
		LEFT JOIN health_check_responses r ON s.id = r.session_id
		WHERE s.assessment_period = $1
		ORDER BY s.date DESC, r.dimension_id
	`, period)
}

// FindTeamHealthByManager retrieves aggregated health data for teams under a manager.
// Prefers post-workshop sessions when available for a team+period, otherwise falls back to individual sessions.
func (r *HealthCheckRepository) FindTeamHealthByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.TeamHealthSummary, error) {
	var query string
	var rows *sql.Rows
	var err error

	// Use effective_sessions CTE to prefer post_workshop data when available
	if assessmentPeriod != "" {
		query = `
			WITH post_workshop_teams AS (
				SELECT DISTINCT s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.survey_type = 'post_workshop'
					AND s.completed = true AND s.assessment_period = $2
			),
			effective_sessions AS (
				SELECT s.id, s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.completed = true AND s.assessment_period = $2
					AND (
						s.survey_type = 'post_workshop'
						OR (s.survey_type = 'individual'
							AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
					)
			),
			team_overall AS (
				SELECT
					t.id AS team_id,
					t.name AS team_name,
					COUNT(DISTINCT es.id) AS submission_count,
					AVG(r.score) AS overall_health,
					CASE WHEN EXISTS (
						SELECT 1 FROM post_workshop_teams pw WHERE pw.team_id = t.id
					) THEN 'submitted' ELSE 'pending' END AS post_workshop_status
				FROM teams t
				INNER JOIN team_supervisors ts ON t.id = ts.team_id
				LEFT JOIN effective_sessions es ON t.id = es.team_id
				LEFT JOIN health_check_responses r ON es.id = r.session_id
				WHERE ts.user_id = $1
				GROUP BY t.id, t.name
			),
			team_dimensions AS (
				SELECT
					es.team_id,
					r.dimension_id,
					AVG(r.score) AS avg_score,
					COUNT(r.dimension_id) AS response_count
				FROM effective_sessions es
				INNER JOIN health_check_responses r ON es.id = r.session_id
				GROUP BY es.team_id, r.dimension_id
			)
			SELECT
				o.team_id,
				o.team_name,
				o.submission_count,
				o.overall_health,
				o.post_workshop_status,
				d.dimension_id,
				d.avg_score,
				d.response_count
			FROM team_overall o
			LEFT JOIN team_dimensions d ON o.team_id = d.team_id
			ORDER BY o.overall_health ASC NULLS LAST, o.team_name, d.dimension_id
		`
		rows, err = r.db.QueryContext(ctx, query, managerID, assessmentPeriod)
	} else {
		query = `
			WITH post_workshop_teams AS (
				SELECT DISTINCT s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.survey_type = 'post_workshop'
					AND s.completed = true
			),
			effective_sessions AS (
				SELECT s.id, s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.completed = true
					AND (
						s.survey_type = 'post_workshop'
						OR (s.survey_type = 'individual'
							AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
					)
			),
			team_overall AS (
				SELECT
					t.id AS team_id,
					t.name AS team_name,
					COUNT(DISTINCT es.id) AS submission_count,
					AVG(r.score) AS overall_health,
					CASE WHEN EXISTS (
						SELECT 1 FROM post_workshop_teams pw WHERE pw.team_id = t.id
					) THEN 'submitted' ELSE 'pending' END AS post_workshop_status
				FROM teams t
				INNER JOIN team_supervisors ts ON t.id = ts.team_id
				LEFT JOIN effective_sessions es ON t.id = es.team_id
				LEFT JOIN health_check_responses r ON es.id = r.session_id
				WHERE ts.user_id = $1
				GROUP BY t.id, t.name
			),
			team_dimensions AS (
				SELECT
					es.team_id,
					r.dimension_id,
					AVG(r.score) AS avg_score,
					COUNT(r.dimension_id) AS response_count
				FROM effective_sessions es
				INNER JOIN health_check_responses r ON es.id = r.session_id
				GROUP BY es.team_id, r.dimension_id
			)
			SELECT
				o.team_id,
				o.team_name,
				o.submission_count,
				o.overall_health,
				o.post_workshop_status,
				d.dimension_id,
				d.avg_score,
				d.response_count
			FROM team_overall o
			LEFT JOIN team_dimensions d ON o.team_id = d.team_id
			ORDER BY o.overall_health ASC NULLS LAST, o.team_name, d.dimension_id
		`
		rows, err = r.db.QueryContext(ctx, query, managerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query team health by manager: %w", err)
	}
	defer rows.Close()

	// Group results by team
	teamsMap := make(map[string]*healthcheck.TeamHealthSummary)
	teamOrder := []string{}

	for rows.Next() {
		var teamID, teamName string
		var submissionCount int
		var overallHealth sql.NullFloat64
		var postWorkshopStatus string
		var dimensionID sql.NullString
		var avgScore sql.NullFloat64
		var responseCount sql.NullInt64

		err := rows.Scan(
			&teamID,
			&teamName,
			&submissionCount,
			&overallHealth,
			&postWorkshopStatus,
			&dimensionID,
			&avgScore,
			&responseCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team health row: %w", err)
		}

		// Create or get team summary
		team, exists := teamsMap[teamID]
		if !exists {
			health := 0.0
			if overallHealth.Valid {
				health = overallHealth.Float64
			}
			team = &healthcheck.TeamHealthSummary{
				TeamID:             teamID,
				TeamName:           teamName,
				SubmissionCount:    submissionCount,
				OverallHealth:      health,
				Dimensions:         []healthcheck.DimensionSummary{},
				PostWorkshopStatus: postWorkshopStatus,
			}
			teamsMap[teamID] = team
			teamOrder = append(teamOrder, teamID)
		}

		// Add dimension summary if exists
		if dimensionID.Valid && avgScore.Valid {
			team.Dimensions = append(team.Dimensions, healthcheck.DimensionSummary{
				DimensionID:   dimensionID.String,
				AvgScore:      avgScore.Float64,
				ResponseCount: int(responseCount.Int64),
			})
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Convert map to ordered slice (order is already by overall_health ASC from query)
	teams := make([]healthcheck.TeamHealthSummary, 0, len(teamOrder))
	for _, id := range teamOrder {
		teams = append(teams, *teamsMap[id])
	}

	return teams, nil
}

// FindAggregatedDimensionsByManager retrieves aggregated dimension data across all teams under a manager.
// Prefers post-workshop sessions when available for a team+period, otherwise falls back to individual sessions.
func (r *HealthCheckRepository) FindAggregatedDimensionsByManager(ctx context.Context, managerID string, assessmentPeriod string) ([]healthcheck.DimensionSummary, error) {
	var query string
	var rows *sql.Rows
	var err error

	if assessmentPeriod != "" {
		query = `
			WITH post_workshop_teams AS (
				SELECT DISTINCT s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.survey_type = 'post_workshop'
					AND s.completed = true AND s.assessment_period = $2
			),
			effective_sessions AS (
				SELECT s.id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.completed = true AND s.assessment_period = $2
					AND (
						s.survey_type = 'post_workshop'
						OR (s.survey_type = 'individual'
							AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
					)
			)
			SELECT
				r.dimension_id,
				AVG(r.score) AS avg_score,
				COUNT(r.dimension_id) AS response_count
			FROM effective_sessions es
			INNER JOIN health_check_responses r ON es.id = r.session_id
			GROUP BY r.dimension_id
			ORDER BY r.dimension_id
		`
		rows, err = r.db.QueryContext(ctx, query, managerID, assessmentPeriod)
	} else {
		query = `
			WITH post_workshop_teams AS (
				SELECT DISTINCT s.team_id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.survey_type = 'post_workshop'
					AND s.completed = true
			),
			effective_sessions AS (
				SELECT s.id
				FROM health_check_sessions s
				INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
				WHERE ts.user_id = $1 AND s.completed = true
					AND (
						s.survey_type = 'post_workshop'
						OR (s.survey_type = 'individual'
							AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
					)
			)
			SELECT
				r.dimension_id,
				AVG(r.score) AS avg_score,
				COUNT(r.dimension_id) AS response_count
			FROM effective_sessions es
			INNER JOIN health_check_responses r ON es.id = r.session_id
			GROUP BY r.dimension_id
			ORDER BY r.dimension_id
		`
		rows, err = r.db.QueryContext(ctx, query, managerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query aggregated dimensions by manager: %w", err)
	}
	defer rows.Close()

	var dimensions []healthcheck.DimensionSummary

	for rows.Next() {
		var dimension healthcheck.DimensionSummary

		err := rows.Scan(
			&dimension.DimensionID,
			&dimension.AvgScore,
			&dimension.ResponseCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan dimension summary: %w", err)
		}

		dimensions = append(dimensions, dimension)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Return empty slice instead of nil
	if dimensions == nil {
		dimensions = []healthcheck.DimensionSummary{}
	}

	return dimensions, nil
}

// GetTeamSubmissionStatus returns submission status for a team in a given assessment period
func (r *HealthCheckRepository) GetTeamSubmissionStatus(ctx context.Context, teamID string, assessmentPeriod string) (*healthcheck.TeamSubmissionStatus, error) {
	query := `
		SELECT
			COUNT(DISTINCT tm.user_id) AS total_members,
			COUNT(DISTINCT CASE WHEN hcs.completed = true THEN hcs.user_id END) AS submitted_members,
			EXISTS(
				SELECT 1 FROM health_check_sessions
				WHERE team_id = $1 AND assessment_period = $2
					AND survey_type = 'post_workshop' AND completed = true
			) AS post_workshop_exists
		FROM team_members tm
		LEFT JOIN health_check_sessions hcs ON tm.user_id = hcs.user_id AND tm.team_id = hcs.team_id
			AND hcs.assessment_period = $2 AND hcs.survey_type = 'individual'
		WHERE tm.team_id = $1
	`

	var totalMembers, submittedMembers int
	var postWorkshopExists bool

	err := r.db.QueryRowContext(ctx, query, teamID, assessmentPeriod).Scan(
		&totalMembers, &submittedMembers, &postWorkshopExists,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query team submission status: %w", err)
	}

	allSubmitted := totalMembers > 0 && submittedMembers >= totalMembers

	return &healthcheck.TeamSubmissionStatus{
		TeamID:             teamID,
		AssessmentPeriod:   assessmentPeriod,
		TotalMembers:       totalMembers,
		SubmittedMembers:   submittedMembers,
		AllSubmitted:       allSubmitted,
		PostWorkshopExists: postWorkshopExists,
	}, nil
}

// Delete removes a session and its responses (cascade handled by DB)
func (r *HealthCheckRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM health_check_sessions WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// FindDistinctAssessmentPeriods returns all unique assessment periods from submitted sessions
func (r *HealthCheckRepository) FindDistinctAssessmentPeriods(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT assessment_period FROM health_check_sessions
		WHERE assessment_period IS NOT NULL AND assessment_period != '' AND completed = true
		ORDER BY assessment_period DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query assessment periods: %w", err)
	}
	defer rows.Close()

	var periods []string
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return nil, fmt.Errorf("failed to scan assessment period: %w", err)
		}
		periods = append(periods, period)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating assessment periods: %w", err)
	}

	if periods == nil {
		periods = []string{}
	}

	return periods, nil
}

// scanSessions is a helper function to scan query results into sessions
func (r *HealthCheckRepository) scanSessions(ctx context.Context, query string, args ...interface{}) ([]*healthcheck.HealthCheckSession, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	sessionsMap := make(map[string]*healthcheck.HealthCheckSession)
	sessionOrder := []string{}

	for rows.Next() {
		var (
			sessionID        string
			teamID           string
			userID           string
			date             string
			assessmentPeriod sql.NullString
			surveyType       sql.NullString
			completed        bool
			dimensionID      sql.NullString
			score            sql.NullInt64
			trend            sql.NullString
			comment          sql.NullString
		)

		err := rows.Scan(
			&sessionID, &teamID, &userID, &date, &assessmentPeriod, &surveyType, &completed,
			&dimensionID, &score, &trend, &comment,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create or get session
		session, exists := sessionsMap[sessionID]
		if !exists {
			st := healthcheck.SurveyTypeIndividual
			if surveyType.Valid && surveyType.String != "" {
				st = surveyType.String
			}
			session = &healthcheck.HealthCheckSession{
				ID:               sessionID,
				TeamID:           teamID,
				UserID:           userID,
				Date:             date,
				AssessmentPeriod: assessmentPeriod.String,
				SurveyType:       st,
				Completed:        completed,
				Responses:        []healthcheck.HealthCheckResponse{},
			}
			sessionsMap[sessionID] = session
			sessionOrder = append(sessionOrder, sessionID)
		}

		// Add response if exists
		if dimensionID.Valid {
			session.Responses = append(session.Responses, healthcheck.HealthCheckResponse{
				DimensionID: dimensionID.String,
				Score:       int(score.Int64),
				Trend:       trend.String,
				Comment:     comment.String,
			})
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Convert map to ordered slice
	sessions := make([]*healthcheck.HealthCheckSession, 0, len(sessionOrder))
	for _, id := range sessionOrder {
		sessions = append(sessions, sessionsMap[id])
	}

	return sessions, nil
}
