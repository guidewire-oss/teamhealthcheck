// Copyright 2025 Guidewire Software, Inc.
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
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Team Results API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		cleanup    func()
		jwtService *services.JWTService
		userToken  string
	)

	BeforeEach(func() {
		// Set Gin to test mode
		gin.SetMode(gin.TestMode)

		// Setup test database with helpers
		db, cleanup = testhelpers.SetupTestDatabase()

		// Initialize JWT service and generate a manager-level token for tests
		// Manager (level-3) can access any team's data via TeamMembershipMiddleware
		jwtService = services.NewJWTService()
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "test_manager", "test_manager", "manager@test.com", "level-3", nil)
		Expect(tokenErr).NotTo(HaveOccurred())
		userToken = tokenPair.AccessToken

		// Initialize router with API routes
		router = gin.New()
		healthCheckRepo := postgres.NewHealthCheckRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		orgRepo := postgres.NewOrganizationRepository(db)

		v1.SetupHealthCheckRoutes(router, healthCheckRepo, orgRepo, jwtService, nil)
		v1.SetupTeamRoutes(router, healthCheckRepo, teamRepo, jwtService)
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("GET /api/v1/teams/:teamId", func() {
		Context("when retrieving team health check sessions", func() {
			It("should return all sessions for the team with responses", func() {
				// Given: Create test data
				currentDate := time.Now().Format("2006-01-02")

				// Create users
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES
						('tr_lead1', 'tr_lead1', 'tr_lead1@test.com', 'Lead One', 'level-4'),
						('tr_mem1', 'tr_mem1', 'tr_mem1@test.com', 'Member One', 'level-5'),
						('tr_mem2', 'tr_mem2', 'tr_mem2@test.com', 'Member Two', 'level-5')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create team
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('tr_alpha', 'Alpha Squad', 'tr_lead1')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('tr_sess1', 'tr_alpha', 'tr_mem1', $1, '2024 - 2nd Half', true),
						('tr_sess2', 'tr_alpha', 'tr_mem2', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for session 1
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('tr_sess1', 'mission', 3, 'improving', 'Great clarity'),
						('tr_sess1', 'value', 2, 'stable', 'Good output'),
						('tr_sess1', 'speed', 3, 'improving', 'Fast delivery')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for session 2
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('tr_sess2', 'mission', 2, 'stable', 'Clear goals'),
						('tr_sess2', 'value', 2, 'improving', 'Delivering well'),
						('tr_sess2', 'speed', 1, 'declining', 'Needs improvement')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Request team sessions
				req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/tr_alpha/sessions", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Response should contain sessions with responses
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				// Verify sessions array exists
				sessions := response["sessions"].([]interface{})
				Expect(sessions).To(HaveLen(2))

				// Verify first session structure
				session1 := sessions[0].(map[string]interface{})
				Expect(session1["id"]).To(Equal("tr_sess1"))
				Expect(session1["teamId"]).To(Equal("tr_alpha"))
				Expect(session1["userId"]).To(Equal("tr_mem1"))
				Expect(session1["completed"]).To(BeTrue())
				Expect(session1["assessmentPeriod"]).To(Equal("2024 - 2nd Half"))

				// Verify responses exist in session
				responses1 := session1["responses"].([]interface{})
				Expect(responses1).To(HaveLen(3))

				// Verify response structure
				response1 := responses1[0].(map[string]interface{})
				Expect(response1).To(HaveKey("dimensionId"))
				Expect(response1).To(HaveKey("score"))
				Expect(response1).To(HaveKey("trend"))
				Expect(response1).To(HaveKey("comment"))

				// Verify specific dimension data
				missionResponse := findResponseByDimension(responses1, "mission")
				Expect(missionResponse).NotTo(BeNil())
				Expect(missionResponse["score"]).To(BeNumerically("==", 3))
				Expect(missionResponse["trend"]).To(Equal("improving"))
				Expect(missionResponse["comment"]).To(Equal("Great clarity"))

				// Verify total count
				Expect(response["total"]).To(BeNumerically("==", 2))
			})

			It("should filter sessions by assessment period", func() {
				// Given: Team with sessions in different periods
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES ('filtermem1', 'filtermem1', 'filtermem1@test.com', 'Member One', 'level-5')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('filterteam', 'Filter Squad', 'filtermem1')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create sessions in different periods
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('filtersess1', 'filterteam', 'filtermem1', '2024-01-15', '2023 - 2nd Half', true),
						('filtersess2', 'filterteam', 'filtermem1', '2024-07-15', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES
						('filtersess1', 'mission', 2, 'stable'),
						('filtersess2', 'mission', 3, 'improving')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Request with period filter
				req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/filterteam/sessions?assessmentPeriod=2024+-+1st+Half", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should only return filtered sessions
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				sessions := response["sessions"].([]interface{})
				Expect(sessions).To(HaveLen(1))

				session := sessions[0].(map[string]interface{})
				Expect(session["id"]).To(Equal("filtersess2"))
				Expect(session["assessmentPeriod"]).To(Equal("2024 - 1st Half"))
			})

			It("should return empty array when team has no sessions", func() {
				// Given: Team with no sessions
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
					VALUES ('emptylead', 'emptylead', 'emptylead@test.com', 'Lead One', 'level-4')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('emptyteam', 'Empty Team', 'emptylead')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Request team sessions
				req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/emptyteam/sessions", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return empty array
				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				sessions := response["sessions"].([]interface{})
				Expect(sessions).To(HaveLen(0))
				total := response["total"]
				if total != nil {
					Expect(total).To(BeNumerically("==", 0))
				}
			})

			It("should return 404 for non-existent route", func() {
				// When: Request to a route that doesn't exist
				req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/nonexistent/nonexistent", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Should return 404 (route not found)
				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})
	})
})

// Helper function
func findResponseByDimension(responses []interface{}, dimensionId string) map[string]interface{} {
	for _, r := range responses {
		response := r.(map[string]interface{})
		if response["dimensionId"] == dimensionId {
			return response
		}
	}
	return nil
}
