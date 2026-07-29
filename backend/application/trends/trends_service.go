// SPDX-License-Identifier: Apache-2.0

package trends

import (
	"context"
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
)

// AllDimensionIDs contains all 11 health check dimension IDs
// This ensures trends always return all dimensions, even if no data exists
var AllDimensionIDs = []string{
	"mission",
	"value",
	"speed",
	"fun",
	"health",
	"learning",
	"support",
	"pawns",
	"release",
	"process",
	"teamwork",
}

// Service provides trend data aggregation functionality
type Service struct {
	db *sql.DB
}

// NewService creates a new trends service
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// TrendResult holds the computed trend data
type TrendResult struct {
	Periods    []string
	Dimensions []dto.DimensionTrend
}

// GetTrendsForTeam returns trend data for a single team
func (s *Service) GetTrendsForTeam(ctx context.Context, teamID string) (*TrendResult, error) {
	// Get distinct assessment periods for this team
	periodsQuery := `
		SELECT DISTINCT assessment_period
		FROM health_check_sessions
		WHERE team_id = $1
			AND completed = true
			AND assessment_period IS NOT NULL
			AND assessment_period != ''
		ORDER BY assessment_period
	`

	periods, err := s.fetchPeriods(ctx, periodsQuery, teamID)
	if err != nil {
		return nil, err
	}

	if len(periods) == 0 {
		return &TrendResult{
			Periods:    []string{},
			Dimensions: []dto.DimensionTrend{},
		}, nil
	}

	// Get average scores per dimension per period
	trendsQuery := `
		SELECT
			hcr.dimension_id,
			hcs.assessment_period,
			AVG(hcr.score) as avg_score
		FROM health_check_responses hcr
		INNER JOIN health_check_sessions hcs ON hcr.session_id = hcs.id
		WHERE hcs.team_id = $1
			AND hcs.completed = true
			AND hcs.assessment_period IS NOT NULL
			AND hcs.assessment_period != ''
		GROUP BY hcr.dimension_id, hcs.assessment_period
		ORDER BY hcr.dimension_id, hcs.assessment_period
	`

	dimensions, err := s.fetchTrendData(ctx, trendsQuery, teamID, periods)
	if err != nil {
		return nil, err
	}

	return &TrendResult{
		Periods:    periods,
		Dimensions: dimensions,
	}, nil
}

// GetTrendsForManager returns aggregated trend data across all teams supervised by a manager.
// Prefers post-workshop sessions when available for a team+period, otherwise falls back to individual sessions.
func (s *Service) GetTrendsForManager(ctx context.Context, managerID string) (*TrendResult, error) {
	// Get distinct assessment periods for supervised teams
	periodsQuery := `
		SELECT DISTINCT hcs.assessment_period
		FROM health_check_sessions hcs
		INNER JOIN teams t ON hcs.team_id = t.id
		INNER JOIN team_supervisors ts ON t.id = ts.team_id
		WHERE ts.user_id = $1
			AND hcs.completed = true
			AND hcs.assessment_period IS NOT NULL
			AND hcs.assessment_period != ''
		ORDER BY hcs.assessment_period
	`

	periods, err := s.fetchPeriods(ctx, periodsQuery, managerID)
	if err != nil {
		return nil, err
	}

	if len(periods) == 0 {
		return &TrendResult{
			Periods:    []string{},
			Dimensions: []dto.DimensionTrend{},
		}, nil
	}

	// Get aggregated average scores per dimension per period across all teams
	// Uses effective_sessions CTE to prefer post-workshop data when available
	trendsQuery := `
		WITH post_workshop_teams AS (
			SELECT DISTINCT hcs.team_id, hcs.assessment_period
			FROM health_check_sessions hcs
			INNER JOIN teams t ON hcs.team_id = t.id
			INNER JOIN team_supervisors ts ON t.id = ts.team_id
			WHERE ts.user_id = $1 AND hcs.survey_type = 'post_workshop'
				AND hcs.completed = true
				AND hcs.assessment_period IS NOT NULL
				AND hcs.assessment_period != ''
		),
		effective_sessions AS (
			SELECT hcs.id, hcs.assessment_period
			FROM health_check_sessions hcs
			INNER JOIN teams t ON hcs.team_id = t.id
			INNER JOIN team_supervisors ts ON t.id = ts.team_id
			WHERE ts.user_id = $1
				AND hcs.completed = true
				AND hcs.assessment_period IS NOT NULL
				AND hcs.assessment_period != ''
				AND (
					hcs.survey_type = 'post_workshop'
					OR (hcs.survey_type = 'individual'
						AND NOT EXISTS (
							SELECT 1 FROM post_workshop_teams pw
							WHERE pw.team_id = hcs.team_id AND pw.assessment_period = hcs.assessment_period
						))
				)
		)
		SELECT
			hcr.dimension_id,
			es.assessment_period,
			AVG(hcr.score) as avg_score
		FROM health_check_responses hcr
		INNER JOIN effective_sessions es ON hcr.session_id = es.id
		GROUP BY hcr.dimension_id, es.assessment_period
		ORDER BY hcr.dimension_id, es.assessment_period
	`

	dimensions, err := s.fetchTrendData(ctx, trendsQuery, managerID, periods)
	if err != nil {
		return nil, err
	}

	return &TrendResult{
		Periods:    periods,
		Dimensions: dimensions,
	}, nil
}

// fetchPeriods executes a periods query and returns the distinct periods
func (s *Service) fetchPeriods(ctx context.Context, query string, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periods := []string{}
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err == nil {
			periods = append(periods, period)
		}
	}

	return periods, nil
}

// fetchTrendData executes a trends query and returns the dimension trends
// Always returns all 11 dimensions, with 0 scores for periods where no data exists
func (s *Service) fetchTrendData(ctx context.Context, query string, id string, periods []string) ([]dto.DimensionTrend, error) {
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Build a map of dimension -> period -> score
	trendMap := make(map[string]map[string]float64)
	for rows.Next() {
		var dimensionID string
		var period string
		var avgScore float64

		if err := rows.Scan(&dimensionID, &period, &avgScore); err != nil {
			continue
		}

		if _, exists := trendMap[dimensionID]; !exists {
			trendMap[dimensionID] = make(map[string]float64)
		}
		trendMap[dimensionID][period] = avgScore
	}

	// Convert to ordered array format matching periods order
	// IMPORTANT: Iterate over AllDimensionIDs to ensure all 11 dimensions are included
	dimensions := []dto.DimensionTrend{}
	for _, dimensionID := range AllDimensionIDs {
		scores := make([]float64, len(periods))

		// Get scores from the map if they exist
		if periodScores, exists := trendMap[dimensionID]; exists {
			for i, period := range periods {
				if score, ok := periodScores[period]; ok {
					scores[i] = score
				} else {
					scores[i] = 0 // No data for this period
				}
			}
		}
		// If dimension has no data at all, scores array remains all zeros

		dimensions = append(dimensions, dto.DimensionTrend{
			DimensionID: dimensionID,
			Scores:      scores,
		})
	}

	return dimensions, nil
}
