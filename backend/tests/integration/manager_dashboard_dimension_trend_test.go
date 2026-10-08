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

var _ = Describe("Integration: Manager Dashboard Dimension Trend", func() {
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
		tokenPair, tokenErr := jwtService.GenerateTokenPair(context.Background(), "int_dt_mgr", "int_dt_mgr", "dt_mgr@test.com", "level-3", nil)
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

	Describe("GET /api/v1/managers/:managerId/teams/health", func() {
		var setupBaseFixtures = func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id)
				VALUES ('int_dt_mgr', 'int_dt_mgr', 'int_dt_mgr@test.com', 'Dimension Trend Manager', 'level-3')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('int_dt_team', 'Dimension Trend Squad', 'int_dt_mgr')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ('int_dt_team', 'int_dt_mgr', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())
		}

		findDimension := func(dims []interface{}, dimensionID string) map[string]interface{} {
			for _, d := range dims {
				dim := d.(map[string]interface{})
				if dim["dimensionId"] == dimensionID {
					return dim
				}
			}
			return nil
		}

		It("should surface the trend from the final completed post-workshop response", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_dt_session_pw', 'int_dt_team', 'int_dt_mgr', '2024-07-20', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES ('int_dt_session_pw', 'fun', 1, 'improving', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_dt_mgr/teams/health?assessmentPeriod=2024+-+1st+Half", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			Expect(teams).To(HaveLen(1))
			team := teams[0].(map[string]interface{})
			dims := team["dimensions"].([]interface{})

			funDim := findDimension(dims, "fun")
			Expect(funDim).NotTo(BeNil())
			Expect(funDim["trend"]).To(Equal("improving"))
		})

		It("should use only the most recent post-workshop session's trend when multiple exist", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES
					('int_dt_session_pw_old', 'int_dt_team', 'int_dt_mgr', '2024-07-01', '2024 - 1st Half', 'post_workshop', true),
					('int_dt_session_pw_new', 'int_dt_team', 'int_dt_mgr', '2024-07-20', '2024 - 1st Half', 'post_workshop', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES
					('int_dt_session_pw_old', 'fun', 2, 'stable', NULL),
					('int_dt_session_pw_new', 'fun', 1, 'declining', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_dt_mgr/teams/health?assessmentPeriod=2024+-+1st+Half", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			team := teams[0].(map[string]interface{})
			dims := team["dimensions"].([]interface{})

			funDim := findDimension(dims, "fun")
			Expect(funDim).NotTo(BeNil())
			Expect(funDim["trend"]).To(Equal("declining"))
		})

		It("should omit trend when only individual (non-post-workshop) sessions exist", func() {
			setupBaseFixtures()

			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, survey_type, completed)
				VALUES ('int_dt_session_indiv', 'int_dt_team', 'int_dt_mgr', '2024-07-15', '2024 - 1st Half', 'individual', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
				VALUES ('int_dt_session_indiv', 'fun', 2, 'stable', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/managers/int_dt_mgr/teams/health?assessmentPeriod=2024+-+1st+Half", nil)
			req.Header.Set("Authorization", "Bearer "+managerToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())

			teams := response["teams"].([]interface{})
			team := teams[0].(map[string]interface{})
			dims := team["dimensions"].([]interface{})

			funDim := findDimension(dims, "fun")
			Expect(funDim).NotTo(BeNil())
			_, hasTrend := funDim["trend"]
			Expect(hasTrend).To(BeFalse())
		})
	})
})
