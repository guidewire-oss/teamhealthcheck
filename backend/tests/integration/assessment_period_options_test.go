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
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

// The period dropdown on every dashboard reads GET /api/v1/assessment-periods,
// and the admin survey dashboard validates its ?assessmentPeriod= against the
// same list. Stored sessions with a legacy / off-cadence period ("2026 Q1",
// "2026 Q3", "2026 Mar") must stay in the table untouched but never be offered.
var _ = Describe("Integration: assessment period options", func() {
	var (
		db      *sql.DB
		cleanup func()
		router  *gin.Engine
		token   string
	)

	get := func(path string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, path, nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService := services.NewJWTService()
		pair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		token = pair.AccessToken

		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		hcRepo := postgres.NewHealthCheckRepository(db)
		router = gin.New()
		v1.SetupHealthCheckRoutes(router, hcRepo, orgRepo, jwtService, nil)
		v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, hcRepo, jwtService)

		_, err = db.Exec(`INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			VALUES ('ap_user', 'ap_user', 'ap@test.com', 'AP User', 'level-5', 'unused')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('ap_team', 'AP Team')`)
		Expect(err).NotTo(HaveOccurred())
		// One completed session per period, valid and invalid alike.
		for i, p := range []string{"2026 H1", "2026 H2", "2026 Q1", "2026 Q3", "2026 Mar"} {
			_, err = db.Exec(`INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
				VALUES ($1, 'ap_team', 'ap_user', NOW(), $2, true)`, "ap_s"+string(rune('0'+i)), p)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() { cleanup() })

	It("offers exactly 2026 H2 and 2026 H1", func() {
		w := get("/api/v1/assessment-periods")
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

		var body struct {
			Periods []string `json:"periods"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Periods).To(Equal([]string{"2026 H2", "2026 H1"}))
	})

	It("lets the admin survey dashboard filter by a valid half-year and rejects the legacy ones", func() {
		for _, p := range []string{"2026 H1", "2026 H2"} {
			Expect(get("/api/v1/admin/survey-completion?assessmentPeriod="+p).Code).To(Equal(http.StatusOK), p)
		}
		for _, p := range []string{"2026%20Q1", "2026%20Q3", "2026%20Mar"} {
			Expect(get("/api/v1/admin/survey-completion?assessmentPeriod="+p).Code).To(Equal(http.StatusBadRequest), p)
		}
	})

	It("does not modify the stored sessions", func() {
		get("/api/v1/assessment-periods")
		get("/api/v1/admin/survey-completion?assessmentPeriod=2026%20H1")

		rows, err := db.Query(`SELECT assessment_period FROM health_check_sessions ORDER BY id`)
		Expect(err).NotTo(HaveOccurred())
		defer rows.Close()
		var got []string
		for rows.Next() {
			var p string
			Expect(rows.Scan(&p)).To(Succeed())
			got = append(got, p)
		}
		Expect(got).To(Equal([]string{"2026 H1", "2026 H2", "2026 Q1", "2026 Q3", "2026 Mar"}))
	})
})
