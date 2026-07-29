// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Manager Dashboard API", func() {
	var (
		db           *sql.DB
		router       *gin.Engine
		cleanup      func()
		jwtService   *services.JWTService
		managerToken string
	)

	BeforeEach(func() {
		// Set Gin to test mode
		gin.SetMode(gin.TestMode)

		// Setup test database with helpers
		db, cleanup = testhelpers.SetupTestDatabase()

		// Initialize JWT service and generate a director-level token for tests
		// Director (level-2) can access any manager's dashboard data
		jwtService = services.NewJWTService()
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "test_director", "test_director", "director@test.com", "level-2", nil)
		Expect(tokenErr).NotTo(HaveOccurred())
		managerToken = tokenPair.AccessToken

		// Initialize router with API routes
		router = gin.New()
		healthCheckRepo := postgres.NewHealthCheckRepository(db)
		orgRepo := postgres.NewOrganizationRepository(db)
		trendsService := trends.NewService(db)

		v1.SetupHealthCheckRoutes(router, healthCheckRepo, orgRepo, jwtService, nil)
		userRepo := postgres.NewUserRepository(db)
		v1.SetupManagerRoutes(router, healthCheckRepo, trendsService, jwtService, userRepo)
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("GET /api/v1/managers/:managerId/teams/health", func() {
		Context("when a manager views their supervised teams", func() {
			It("should return aggregated health metrics for all supervised teams", func() {
				// Given: Create organizational hierarchy
				// Manager oversees 2 teams with different health check submissions

				// Create users (with int_ prefix to avoid conflict with seed data)
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to)
					VALUES
						('int_manager1', 'int_manager1', 'int_manager1@test.com', 'Manager One', 'level-3', NULL),
						('int_lead1', 'int_lead1', 'int_lead1@test.com', 'Lead One', 'level-4', 'int_manager1'),
						('int_lead2', 'int_lead2', 'int_lead2@test.com', 'Lead Two', 'level-4', 'int_manager1'),
						('int_member1', 'int_member1', 'int_member1@test.com', 'Member One', 'level-5', 'int_lead1'),
						('int_member2', 'int_member2', 'int_member2@test.com', 'Member Two', 'level-5', 'int_lead1'),
						('int_member3', 'int_member3', 'int_member3@test.com', 'Member Three', 'level-5', 'int_lead2')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create teams
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES
						('int_team1', 'Alpha Squad', 'int_lead1'),
						('int_team2', 'Beta Squad', 'int_lead2')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add team members
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES
						('int_team1', 'int_lead1'),
						('int_team1', 'int_member1'),
						('int_team1', 'int_member2'),
						('int_team2', 'int_lead2'),
						('int_team2', 'int_member3')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add supervisor chains
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES
						('int_team1', 'int_lead1', 'level-4', 1),
						('int_team1', 'int_manager1', 'level-3', 2),
						('int_team2', 'int_lead2', 'level-4', 1),
						('int_team2', 'int_manager1', 'level-3', 2)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions for Team 1 (good health)
				currentDate := time.Now().Format("2006-01-02")
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('int_session1', 'int_team1', 'int_member1', $1, '2024 - 2nd Half', true),
						('int_session2', 'int_team1', 'int_member2', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for Team 1 (average score ~2.5 - good)
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('int_session1', 'mission', 3, 'improving', 'Clear goals'),
						('int_session1', 'value', 2, 'stable', 'Good value'),
						('int_session2', 'mission', 3, 'stable', 'Clear mission'),
						('int_session2', 'value', 2, 'improving', 'Delivering well')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions for Team 2 (needs support)
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('int_session3', 'int_team2', 'int_member3', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for Team 2 (average score ~1.5 - needs support)
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('int_session3', 'mission', 2, 'declining', 'Unclear direction'),
						('int_session3', 'value', 1, 'declining', 'Struggling')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager requests their dashboard
				req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_manager1/teams/health", nil)
				req.Header.Set("Authorization", "Bearer "+managerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Response should contain aggregated team health
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				// Verify manager ID
				Expect(response["managerId"]).To(Equal("int_manager1"))

				// Verify we have 2 teams
				teams := response["teams"].([]interface{})
				Expect(teams).To(HaveLen(2))

				// Verify Team 1 (Alpha Squad) - better health
				team1 := findTeamByName(teams, "Alpha Squad")
				Expect(team1).NotTo(BeNil())
				Expect(team1["teamId"]).To(Equal("int_team1"))
				Expect(team1["submissionCount"]).To(BeNumerically("==", 2))

				// Average of Team 1: (3+2+3+2)/4 = 2.5
				team1Health := team1["overallHealth"].(float64)
				Expect(team1Health).To(BeNumerically("~", 2.5, 0.1))

				// Verify Team 2 (Beta Squad) - needs support
				team2 := findTeamByName(teams, "Beta Squad")
				Expect(team2).NotTo(BeNil())
				Expect(team2["teamId"]).To(Equal("int_team2"))
				Expect(team2["submissionCount"]).To(BeNumerically("==", 1))

				// Average of Team 2: (2+1)/2 = 1.5
				team2Health := team2["overallHealth"].(float64)
				Expect(team2Health).To(BeNumerically("~", 1.5, 0.1))

				// Verify teams are sorted by health (worst first for attention)
				firstTeam := teams[0].(map[string]interface{})
				Expect(firstTeam["teamName"]).To(Equal("Beta Squad")) // Lower health comes first

				// Verify dimension-level aggregation
				team1Dimensions := team1["dimensions"].([]interface{})
				Expect(team1Dimensions).To(HaveLen(2)) // mission and value

				missionDim := findDimensionById(team1Dimensions, "mission")
				Expect(missionDim).NotTo(BeNil())
				Expect(missionDim["avgScore"]).To(BeNumerically("==", 3.0)) // (3+3)/2
				Expect(missionDim["responseCount"]).To(BeNumerically("==", 2))

				// Verify total count
				Expect(response["totalTeams"]).To(BeNumerically("==", 2))
			})

			It("should filter by assessment period", func() {
				// Given: Manager with teams that have health checks in different periods

				// Create manager and team (with int_ prefix to avoid seed data conflicts)
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES ('int_mgr2', 'int_mgr2', 'int_mgr2@test.com', 'Manager Two', 'level-3')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('int_team_filter', 'Alpha Squad', 'int_mgr2')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('int_team_filter', 'int_mgr2', 'level-3', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create sessions in different periods
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('int_sess_filter1', 'int_team_filter', 'int_mgr2', '2024-01-15', '2023 - 2nd Half', true),
						('int_sess_filter2', 'int_team_filter', 'int_mgr2', '2024-07-15', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES
						('int_sess_filter1', 'mission', 2, 'stable'),
						('int_sess_filter2', 'mission', 3, 'improving')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager filters by specific period
				req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mgr2/teams/health?assessmentPeriod=2024+-+1st+Half", nil)
				req.Header.Set("Authorization", "Bearer "+managerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should only include data from that period
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				Expect(response["assessmentPeriod"]).To(Equal("2024 - 1st Half"))

				teams := response["teams"].([]interface{})
				team1 := teams[0].(map[string]interface{})

				// Should only have 1 submission from 2024 - 1st Half
				Expect(team1["submissionCount"]).To(BeNumerically("==", 1))

				dimensions := team1["dimensions"].([]interface{})
				mission := dimensions[0].(map[string]interface{})
				Expect(mission["avgScore"]).To(BeNumerically("==", 3.0)) // Only session2's score
			})

			It("should only return teams in the manager's hierarchy", func() {
				// Given: Two managers with different teams (with int_ prefix to avoid seed data conflicts)

				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES
						('int_mgr3', 'int_mgr3', 'int_mgr3@test.com', 'Manager Three', 'level-3'),
						('int_mgr4', 'int_mgr4', 'int_mgr4@test.com', 'Manager Four', 'level-3')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES
						('int_team_mgr3', 'Manager 3 Team', 'int_mgr3'),
						('int_team_mgr4', 'Manager 4 Team', 'int_mgr4')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES
						('int_team_mgr3', 'int_mgr3', 'level-3', 1),
						('int_team_mgr4', 'int_mgr4', 'level-3', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add health check sessions for both teams
				currentDate := time.Now().Format("2006-01-02")
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('int_sess_mgr3', 'int_team_mgr3', 'int_mgr3', $1, '2024 - 2nd Half', true),
						('int_sess_mgr4', 'int_team_mgr4', 'int_mgr4', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES
						('int_sess_mgr3', 'mission', 3, 'improving'),
						('int_sess_mgr4', 'mission', 2, 'stable')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager 3 requests dashboard
				req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mgr3/teams/health", nil)
				req.Header.Set("Authorization", "Bearer "+managerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should only see their own teams (not manager 4's team)
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				teams := response["teams"].([]interface{})
				Expect(teams).To(HaveLen(1))

				team := teams[0].(map[string]interface{})
				Expect(team["teamName"]).To(Equal("Manager 3 Team"))
			})
		})
	})

	Describe("GET /api/v1/managers/:managerId/subordinates", func() {
		Context("when a manager requests their subordinate tree", func() {
			It("should return direct and indirect reports with team memberships", func() {
				// Given: A manager with a hierarchy beneath them
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to)
					VALUES
						('int_sub_mgr', 'int_sub_mgr', 'int_sub_mgr@test.com', 'Sub Manager', 'level-3', NULL),
						('int_sub_lead', 'int_sub_lead', 'int_sub_lead@test.com', 'Sub Lead', 'level-4', 'int_sub_mgr'),
						('int_sub_member', 'int_sub_member', 'int_sub_member@test.com', 'Sub Member', 'level-5', 'int_sub_lead')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('int_sub_team', 'Sub Team', 'int_sub_lead')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES ('int_sub_team', 'int_sub_lead'), ('int_sub_team', 'int_sub_member')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager requests subordinates
				req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_sub_mgr/subordinates", nil)
				req.Header.Set("Authorization", "Bearer "+managerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 with both subordinates
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				Expect(response["managerId"]).To(Equal("int_sub_mgr"))

				subs := response["subordinates"].([]interface{})
				Expect(subs).To(HaveLen(2)) // lead + member

				// Verify subordinates have expected fields
				sub0 := subs[0].(map[string]interface{})
				Expect(sub0).To(HaveKey("id"))
				Expect(sub0).To(HaveKey("name"))
				Expect(sub0).To(HaveKey("hierarchyLevelId"))
				Expect(sub0).To(HaveKey("teamIds"))
				// Email should NOT be returned (PII minimization)
				Expect(sub0).NotTo(HaveKey("email"))

				// Verify team membership is included
				leadSub := findSubordinateById(subs, "int_sub_lead")
				Expect(leadSub).NotTo(BeNil())
				Expect(leadSub["teamIds"]).To(ContainElement("int_sub_team"))
			})

			It("should return empty list when manager has no subordinates", func() {
				// Given: A manager with no reports
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES ('int_lonely_mgr', 'int_lonely_mgr', 'int_lonely@test.com', 'Lonely Manager', 'level-3')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager requests subordinates
				req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_lonely_mgr/subordinates", nil)
				req.Header.Set("Authorization", "Bearer "+managerToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 with empty subordinates
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				subs := response["subordinates"].([]interface{})
				Expect(subs).To(BeEmpty())
			})
		})
	})
})

// Helper functions
func findTeamByName(teams []interface{}, name string) map[string]interface{} {
	for _, t := range teams {
		team := t.(map[string]interface{})
		if team["teamName"] == name {
			return team
		}
	}
	return nil
}

func findDimensionById(dimensions []interface{}, id string) map[string]interface{} {
	for _, d := range dimensions {
		dim := d.(map[string]interface{})
		if dim["dimensionId"] == id {
			return dim
		}
	}
	return nil
}

func findSubordinateById(subs []interface{}, id string) map[string]interface{} {
	for _, s := range subs {
		sub := s.(map[string]interface{})
		if sub["id"] == id {
			return sub
		}
	}
	return nil
}
