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

var _ = Describe("Integration: Manager Dashboard Final Post-Workshop Comments API", func() {
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
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "test_director", "test_director", "director@test.com", "level-2", nil)
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

	Describe("GET /api/v1/managers/:managerId/dashboard/final-post-workshop-comments", func() {
		var setupBaseFixtures = func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
				VALUES ('int_pwc_mgr', 'int_pwc_mgr', 'int_pwc_mgr@test.com', 'PWC Manager', 'level-3')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_pwc_team', 'PWC Squad', 'int_pwc_mgr')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_pwc_team', 'int_pwc_mgr', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())
		}

		It("should return comments from a completed final post-workshop survey", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_pwc_session1', 'int_pwc_team', 'int_pwc_mgr', '2024-07-15', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_pwc_session1', 'mission', 3, 'improving', 'Great alignment this cycle'),
					('int_pwc_session1', 'value', 2, 'stable', 'Delivering steadily')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_pwc_mgr/dashboard/final-post-workshop-comments", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			comments := response["comments"].(map[string]interface{})
			teamComments := comments["int_pwc_team"].([]interface{})
			Expect(teamComments).To(HaveLen(2))

			first := teamComments[0].(map[string]interface{})
			Expect(first["teamId"]).To(Equal("int_pwc_team"))
			Expect(first["comment"]).NotTo(BeEmpty())
		})

		It("should filter comments by assessment period", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES
					('int_pwc_session_old', 'int_pwc_team', 'int_pwc_mgr', '2024-01-15', '2023 - 2nd Half', 'post_workshop', true),
					('int_pwc_session_new', 'int_pwc_team', 'int_pwc_mgr', '2024-07-15', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_pwc_session_old', 'mission', 2, 'stable', 'Old period comment'),
					('int_pwc_session_new', 'mission', 3, 'improving', 'New period comment')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_pwc_mgr/dashboard/final-post-workshop-comments?assessmentPeriod=2024+-+1st+Half", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			comments := response["comments"].(map[string]interface{})
			teamComments := comments["int_pwc_team"].([]interface{})
			Expect(teamComments).To(HaveLen(1))
			only := teamComments[0].(map[string]interface{})
			Expect(only["comment"]).To(Equal("New period comment"))
		})

		It("should exclude comments from individual (non-final) surveys", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_pwc_session_indiv', 'int_pwc_team', 'int_pwc_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES ('int_pwc_session_indiv', 'mission', 3, 'improving', 'Individual survey comment')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_pwc_mgr/dashboard/final-post-workshop-comments", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			comments := response["comments"].(map[string]interface{})
			Expect(comments).To(BeEmpty())
		})

		It("should exclude null and whitespace-only comments", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_pwc_session_blank', 'int_pwc_team', 'int_pwc_mgr', '2024-07-15', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_pwc_session_blank', 'mission', 3, 'improving', NULL),
					('int_pwc_session_blank', 'value', 2, 'stable', '   ')
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_pwc_mgr/dashboard/final-post-workshop-comments", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			comments := response["comments"].(map[string]interface{})
			Expect(comments).To(BeEmpty())
		})

		It("should not leak comments from a team supervised by a different manager", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
				VALUES ('int_pwc_mgr_other', 'int_pwc_mgr_other', 'int_pwc_mgr_other@test.com', 'Other Manager', 'level-3')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_pwc_team_other', 'Other Squad', 'int_pwc_mgr_other')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_pwc_team_other', 'int_pwc_mgr_other', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_pwc_session_other', 'int_pwc_team_other', 'int_pwc_mgr_other', '2024-07-15', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES ('int_pwc_session_other', 'mission', 3, 'improving', 'Comment only for other manager')
			`)
			Expect(err).NotTo(HaveOccurred())

			// Manager int_pwc_mgr has no post-workshop data of their own here
			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_pwc_mgr/dashboard/final-post-workshop-comments", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			comments := response["comments"].(map[string]interface{})
			Expect(comments).NotTo(HaveKey("int_pwc_team_other"))
			Expect(comments).To(BeEmpty())
		})
	})
})
