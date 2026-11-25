package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

// PIT-199: PodIQ owns enrollment (healthCheckEnabled via the organization sync). THC must
// refuse new submissions for a disabled team through the real route stack, keep history,
// and refuse local edits of the flag.
var _ = Describe("Integration: health check participation gate (PIT-199)", func() {
	var (
		db         *sql.DB
		cleanup    func()
		router     *gin.Engine
		adminToken string
		memberTok  string
		hcRepo     healthcheck.Repository
	)

	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// The API takes the storage format ("2026 H2"); ActiveHalfYearPeriod returns the display
	// format ("H2 2026"), which the endpoints reject.
	activePeriod := func() string {
		half, year := healthcheck.HalfYearOf(time.Now())
		return fmt.Sprintf("%d H%d", year, half)
	}

	submitBody := func(teamID, surveyType string) string {
		period := activePeriod()
		return fmt.Sprintf(`{"teamId":%q,"userId":"pit199_member","date":%q,"assessmentPeriod":%q,"surveyType":%q,"completed":true,
			"responses":[{"dimensionId":"mission","score":3,"trend":"stable"}]}`,
			teamID, time.Now().Format("2006-01-02T15:04:05Z07:00"), period, surveyType)
	}

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		db, cleanup = testhelpers.SetupTestDatabase()
		jwtService := services.NewJWTService()
		pair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = pair.AccessToken
		mp, err := jwtService.GenerateTokenPair(context.Background(), "pit199_member", "pit199_member", "m@test.com", "level-5", []string{"pit199_on", "pit199_off"})
		Expect(err).NotTo(HaveOccurred())
		memberTok = mp.AccessToken

		_, err = db.Exec(`INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			VALUES ('pit199_member','pit199_member','m@test.com','Member','level-5','unused')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO teams (id, name, health_check_enabled) VALUES
			('pit199_on','On',true), ('pit199_off','Off',false)`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('pit199_on','pit199_member'),('pit199_off','pit199_member')`)
		Expect(err).NotTo(HaveOccurred())

		router = gin.New()
		orgRepo := postgres.NewOrganizationRepository(db)
		hcRepo = postgres.NewHealthCheckRepository(db)
		v1.SetupHealthCheckRoutes(router, hcRepo, orgRepo, jwtService, nil)
		v1.SetupAdminRoutes(router, orgRepo, postgres.NewUserRepository(db), postgres.NewTeamRepository(db), hcRepo, jwtService)
	})

	AfterEach(func() { cleanup() })

	It("blocks individual and post-workshop submissions for a disabled team, persisting nothing", func() {
		for _, st := range []string{"individual", "post_workshop"} {
			w := call(http.MethodPost, "/api/v1/health-checks", memberTok, submitBody("pit199_off", st))
			Expect(w.Code).To(Equal(http.StatusForbidden), w.Body.String())
			var resp dto.ErrorResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp.Code).To(Equal(healthcheck.CodeParticipationDisabled))
		}
		var n int
		Expect(db.QueryRow(`SELECT COUNT(*) FROM health_check_sessions WHERE team_id='pit199_off'`).Scan(&n)).To(Succeed())
		Expect(n).To(Equal(0))
	})

	It("still accepts an enabled team (existing period and duplicate rules apply)", func() {
		w := call(http.MethodPost, "/api/v1/health-checks", memberTok, submitBody("pit199_on", "individual"))
		Expect(w.Code).To(Equal(http.StatusCreated), w.Body.String())
		w = call(http.MethodPost, "/api/v1/health-checks", memberTok, submitBody("pit199_on", "individual"))
		Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
	})

	It("rejects the save itself for a disabled team, even if the command's earlier check passed", func() {
		// Simulates the window between the command's participation read and the insert: the
		// repository is handed a session directly, so only its own in-transaction check can stop it.
		err := hcRepo.Save(context.Background(), &healthcheck.HealthCheckSession{
			ID:               "pit199_race_session",
			TeamID:           "pit199_off",
			UserID:           "pit199_member",
			Date:             time.Now().Format("2006-01-02T15:04:05Z07:00"),
			AssessmentPeriod: activePeriod(),
			SurveyType:       healthcheck.SurveyTypeIndividual,
			Completed:        true,
			Responses:        []healthcheck.HealthCheckResponse{{DimensionID: "mission", Score: 3, Trend: "stable"}},
		})
		var disabled *healthcheck.ParticipationDisabledError
		Expect(errors.As(err, &disabled)).To(BeTrue(), "expected ParticipationDisabledError, got %v", err)

		var n int
		Expect(db.QueryRow(`SELECT COUNT(*) FROM health_check_sessions WHERE id='pit199_race_session'`).Scan(&n)).To(Succeed())
		Expect(n).To(Equal(0))
	})

	It("keeps history after opt-out and reports the team ineligible", func() {
		w := call(http.MethodPost, "/api/v1/health-checks", memberTok, submitBody("pit199_on", "individual"))
		Expect(w.Code).To(Equal(http.StatusCreated), w.Body.String())
		_, err := db.Exec(`UPDATE teams SET health_check_enabled = false WHERE id = 'pit199_on'`) // what a sync of explicit false writes
		Expect(err).NotTo(HaveOccurred())

		var n int
		Expect(db.QueryRow(`SELECT COUNT(*) FROM health_check_sessions WHERE team_id='pit199_on'`).Scan(&n)).To(Succeed())
		Expect(n).To(Equal(1))

		q := fmt.Sprintf("/api/v1/health-checks/eligibility?surveyType=individual&userId=pit199_member&teamId=pit199_on&assessmentPeriod=%s",
			url.QueryEscape(activePeriod()))
		w = call(http.MethodGet, q, memberTok, "")
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
		var elig dto.SurveyEligibilityResponse
		Expect(json.Unmarshal(w.Body.Bytes(), &elig)).To(Succeed())
		Expect(elig.Eligible).To(BeFalse())
		Expect(elig.Reason).To(Equal(healthcheck.CodeParticipationDisabled))
	})

	It("rejects a local edit of participation and applies no other field of that request", func() {
		w := call(http.MethodPut, "/api/v1/admin/teams/pit199_on", adminToken, `{"name":"Renamed","healthCheckEnabled":false}`)
		Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
		var resp dto.ErrorResponse
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Code).To(Equal(dto.CodeParticipationManagedByPodIQ))

		var name string
		var enabled bool
		Expect(db.QueryRow(`SELECT name, health_check_enabled FROM teams WHERE id='pit199_on'`).Scan(&name, &enabled)).To(Succeed())
		Expect(name).To(Equal("On"))
		Expect(enabled).To(BeTrue())
	})

	It("still allows an ordinary team edit that omits participation", func() {
		w := call(http.MethodPut, "/api/v1/admin/teams/pit199_on", adminToken, `{"name":"Renamed"}`)
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
	})
})
