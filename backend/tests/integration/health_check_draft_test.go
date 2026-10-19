package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Health Check Draft & Submission API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		cleanup    func()
		jwtService *services.JWTService
		repo       healthcheck.Repository
		userAToken string
		userBToken string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService = services.NewJWTService()
		tokenA, err := jwtService.GenerateTokenPair(context.Background(), "int_draft_userA", "int_draft_userA", "usera@test.com", "level-5", []string{"int_draft_team"})
		Expect(err).NotTo(HaveOccurred())
		userAToken = tokenA.AccessToken

		tokenB, err := jwtService.GenerateTokenPair(context.Background(), "int_draft_userB", "int_draft_userB", "userb@test.com", "level-5", []string{"int_draft_team"})
		Expect(err).NotTo(HaveOccurred())
		userBToken = tokenB.AccessToken

		router = gin.New()
		healthCheckRepo := postgres.NewHealthCheckRepository(db)
		repo = healthCheckRepo
		orgRepo := postgres.NewOrganizationRepository(db)
		v1.SetupHealthCheckRoutes(router, healthCheckRepo, orgRepo, jwtService, nil)
	})

	AfterEach(func() {
		cleanup()
	})

	doRequest := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body != nil {
			b, err := json.Marshal(body)
			Expect(err).NotTo(HaveOccurred())
			reader = bytes.NewReader(b)
		} else {
			reader = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, path, reader)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	Describe("POST /api/v1/health-checks (submission identity & draft cleanup)", func() {
		submitBody := func(userID string, completed bool) map[string]any {
			return map[string]any{
				"teamId":           "int_draft_team",
				"userId":           userID,
				"date":             "2024-01-15T10:00:00Z",
				"assessmentPeriod": "2023 - 2nd Half",
				"surveyType":       "individual",
				"completed":        completed,
				"responses": []map[string]any{
					{"dimensionId": "mission", "score": 3, "trend": "improving", "comment": ""},
				},
			}
		}

		Context("when the request's userId does not match the authenticated user", func() {
			It("should reject with 403 and must not touch any draft or session", func() {
				w := doRequest(http.MethodPost, "/api/v1/health-checks", userAToken, submitBody("int_draft_userB", true))
				Expect(w.Code).To(Equal(http.StatusForbidden))

				var resp map[string]any
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["error"]).NotTo(BeEmpty())

				var count int
				Expect(db.QueryRow(`SELECT COUNT(*) FROM health_check_sessions WHERE user_id = 'int_draft_userB'`).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(0))
			})
		})

		Context("when a completed submission succeeds and a matching draft exists", func() {
			It("should delete the draft via the real HTTP submission path", func() {
				// Given: a saved draft for this user/team/surveyType
				err := repo.SaveDraft(context.Background(), &healthcheck.HealthCheckDraft{
					TeamID: "int_draft_team", UserID: "int_draft_userA", SurveyType: healthcheck.SurveyTypeIndividual,
					AssessmentPeriod: "2023 - 2nd Half", CurrentDimension: 0,
					Responses: []healthcheck.HealthCheckResponse{{DimensionID: "mission", Score: 2}},
				})
				Expect(err).NotTo(HaveOccurred())

				_, getErr := repo.GetDraft(context.Background(), "int_draft_userA", "int_draft_team", healthcheck.SurveyTypeIndividual)
				Expect(getErr).NotTo(HaveOccurred())

				// When: submitting a completed survey through the real HTTP handler
				w := doRequest(http.MethodPost, "/api/v1/health-checks", userAToken, submitBody("int_draft_userA", true))
				Expect(w.Code).To(Equal(http.StatusCreated))

				// Then: the draft is gone
				_, err = repo.GetDraft(context.Background(), "int_draft_userA", "int_draft_team", healthcheck.SurveyTypeIndividual)
				Expect(err).To(MatchError(healthcheck.ErrDraftNotFound))
			})
		})

		Context("when a submission is incomplete (Completed=false)", func() {
			It("should preserve the existing draft", func() {
				err := repo.SaveDraft(context.Background(), &healthcheck.HealthCheckDraft{
					TeamID: "int_draft_team", UserID: "int_draft_userA", SurveyType: healthcheck.SurveyTypeIndividual,
					AssessmentPeriod: "2023 - 2nd Half", CurrentDimension: 0,
					Responses: []healthcheck.HealthCheckResponse{{DimensionID: "mission", Score: 2}},
				})
				Expect(err).NotTo(HaveOccurred())

				w := doRequest(http.MethodPost, "/api/v1/health-checks", userAToken, submitBody("int_draft_userA", false))
				Expect(w.Code).To(Equal(http.StatusCreated))

				retrieved, err := repo.GetDraft(context.Background(), "int_draft_userA", "int_draft_team", healthcheck.SurveyTypeIndividual)
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved).NotTo(BeNil())
			})
		})
	})

	Describe("PUT/GET /api/v1/health-checks/draft (identity, conflicts, error shape)", func() {
		Context("when saving a draft for another user", func() {
			It("should reject with 403", func() {
				w := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userB",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": 0,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 1}},
				})
				Expect(w.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when a second save follows the first", func() {
			It("should always overwrite (last write wins), never rejecting with a conflict", func() {
				first := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userA",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": 0,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 1}},
				})
				Expect(first.Code).To(Equal(http.StatusOK))

				second := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userA",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": 3,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 3}},
				})
				Expect(second.Code).To(Equal(http.StatusOK))

				var body map[string]any
				Expect(json.Unmarshal(second.Body.Bytes(), &body)).To(Succeed())
				Expect(body["currentDimension"]).To(Equal(float64(3)))
			})
		})

		Context("when the request has an invalid trend value", func() {
			It("should return 400 with a safe validation message, never a raw server error", func() {
				w := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userA",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": 0,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 1, "trend": "sideways"}},
				})
				Expect(w.Code).To(Equal(http.StatusBadRequest))
				Expect(w.Body.String()).NotTo(ContainSubstring("sql"))
				Expect(w.Body.String()).NotTo(ContainSubstring("pq:"))
			})
		})

		Context("when the request has a negative currentDimension", func() {
			It("should return 400", func() {
				w := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userA",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": -1,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 1}},
				})
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("when the request has a currentDimension far outside the active dimension range", func() {
			It("should return 400", func() {
				w := doRequest(http.MethodPut, "/api/v1/health-checks/draft", userAToken, map[string]any{
					"teamId": "int_draft_team", "userId": "int_draft_userA",
					"assessmentPeriod": "2023 - 2nd Half", "currentDimension": 9999,
					"responses": []map[string]any{{"dimensionId": "mission", "score": 1}},
				})
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("when fetching another user's draft", func() {
			It("should reject with 403", func() {
				w := doRequest(http.MethodGet, "/api/v1/health-checks/draft?teamId=int_draft_team&userId=int_draft_userA", userBToken, nil)
				Expect(w.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when no draft exists", func() {
			It("should return 404 without leaking internals", func() {
				w := doRequest(http.MethodGet, "/api/v1/health-checks/draft?teamId=int_draft_team_none&userId=int_draft_userA", userAToken, nil)
				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})
	})
})
