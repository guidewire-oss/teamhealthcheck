package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Manager Dashboard Member Overview API", func() {
	var (
		db           *sql.DB
		router       *gin.Engine
		cleanup      func()
		jwtService   *services.JWTService
		managerToken string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService = services.NewJWTService()
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "int_mo_mgr", "int_mo_mgr", "mo_mgr@test.com", "level-3", nil)
		Expect(tokenErr).NotTo(HaveOccurred())
		managerToken = tokenPair.AccessToken

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

	Describe("GET /api/v1/managers/:managerId/dashboard/member-overview", func() {
		var setupBaseFixtures = func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
				VALUES ('int_mo_mgr', 'int_mo_mgr', 'int_mo_mgr@test.com', 'Member Overview Manager', 'level-3')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_mo_team', 'Member Overview Squad', 'int_mo_mgr')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_mo_team', 'int_mo_mgr', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())
		}

		It("should aggregate only individual member survey responses", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_mo_session1', 'int_mo_team', 'int_mo_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_mo_session1', 'mission', 3, 'improving', NULL),
					('int_mo_session1', 'value', 1, 'stable', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mo_mgr/dashboard/member-overview", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			Expect(team["teamId"]).To(Equal("int_mo_team"))
			Expect(team["overallHealth"]).To(BeNumerically("~", 2.0, 0.01))
			Expect(team["submissionCount"]).To(BeNumerically("==", 1))
		})

		It("should exclude post-workshop sessions and data even when they exist for the team", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES
					('int_mo_session_indiv', 'int_mo_team', 'int_mo_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true),
					('int_mo_session_pw', 'int_mo_team', 'int_mo_mgr', '2024-07-20', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_mo_session_indiv', 'mission', 2, 'stable', NULL),
					('int_mo_session_pw', 'mission', 3, 'improving', 'Final workshop comment')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mo_mgr/dashboard/member-overview", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			// Only the individual session's score (2) should count, not the post-workshop one (3)
			Expect(team["overallHealth"]).To(BeNumerically("~", 2.0, 0.01))
			Expect(team["submissionCount"]).To(BeNumerically("==", 1))
		})

		It("should scope results to a single team when teamId is provided", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_mo_team_2', 'Second Squad', 'int_mo_mgr')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_mo_team_2', 'int_mo_mgr', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES
					('int_mo_session_t1', 'int_mo_team', 'int_mo_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true),
					('int_mo_session_t2', 'int_mo_team_2', 'int_mo_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_mo_session_t1', 'mission', 3, 'improving', NULL),
					('int_mo_session_t2', 'mission', 1, 'declining', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mo_mgr/dashboard/member-overview?teamId=int_mo_team_2", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			Expect(team["teamId"]).To(Equal("int_mo_team_2"))
			Expect(team["overallHealth"]).To(BeNumerically("~", 1.0, 0.01))
		})

		It("should filter by assessment period", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES
					('int_mo_session_old', 'int_mo_team', 'int_mo_mgr', '2024-01-15', '2023 - 2nd Half', 'individual', true),
					('int_mo_session_new', 'int_mo_team', 'int_mo_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_mo_session_old', 'mission', 1, 'declining', NULL),
					('int_mo_session_new', 'mission', 3, 'improving', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mo_mgr/dashboard/member-overview?assessmentPeriod=2024+-+1st+Half", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			Expect(team["overallHealth"]).To(BeNumerically("~", 3.0, 0.01))
			Expect(team["submissionCount"]).To(BeNumerically("==", 1))
		})

		It("should not leak data from a team supervised by a different manager", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
				VALUES ('int_mo_mgr_other', 'int_mo_mgr_other', 'int_mo_mgr_other@test.com', 'Other Manager', 'level-3')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_mo_team_other', 'Other Squad', 'int_mo_mgr_other')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_mo_team_other', 'int_mo_mgr_other', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_mo_session_other', 'int_mo_team_other', 'int_mo_mgr_other', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES ('int_mo_session_other', 'mission', 3, 'improving', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_mo_mgr/dashboard/member-overview", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			Expect(team["teamId"]).To(Equal("int_mo_team"))
		})
	})
})
