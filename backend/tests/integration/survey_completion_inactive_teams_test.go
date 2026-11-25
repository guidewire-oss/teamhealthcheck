package integration_test

import (
	"bytes"
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
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

// Regression: a deactivated team (teams.is_active = false) that still has
// health_check_enabled = true must not count as opted in, appear in the
// dashboard lists, or be a reminder candidate. Only active + enabled teams
// participate; active + disabled teams stay "opted out".
var _ = Describe("Integration: Survey Completion excludes inactive teams", func() {
	var (
		db         *sql.DB
		cleanup    func()
		router     *gin.Engine
		mailer     *testhelpers.MockEmailService
		adminToken string
	)

	request := func(method, path, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	collectTeamIDs := func(groups []dto.SurveyCompletionGroupDTO) []string {
		var ids []string
		var walk func(g dto.SurveyCompletionGroupDTO)
		walk = func(g dto.SurveyCompletionGroupDTO) {
			for _, t := range g.DirectTeams {
				ids = append(ids, t.TeamID)
			}
			for _, t := range g.Teams {
				ids = append(ids, t.TeamID)
			}
			for _, c := range g.Children {
				walk(c)
			}
		}
		for _, g := range groups {
			walk(g)
		}
		return ids
	}

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService := services.NewJWTService()
		pair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = pair.AccessToken

		router = gin.New()
		mailer = testhelpers.NewMockEmailService()
		v1.SetupAdminRoutes(router,
			postgres.NewOrganizationRepository(db),
			postgres.NewUserRepository(db),
			postgres.NewTeamRepository(db),
			postgres.NewHealthCheckRepository(db),
			jwtService,
			v1.WithReminderSender(mailer))

		for _, id := range []string{"ia_lead", "ii_lead", "ad_lead"} {
			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1, $1, $2, $1, 'level-4', 'unused')`, id, id+"@test.com")
			Expect(err).NotTo(HaveOccurred())
		}
		// active + enabled, inactive + enabled, active + disabled.
		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id, health_check_enabled, is_active) VALUES
				('ia_team', 'Active Enabled',   'ia_lead', true,  true),
				('ii_team', 'Inactive Enabled', 'ii_lead', true,  false),
				('ad_team', 'Active Disabled',  'ad_lead', false, true)`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`
			INSERT INTO team_members (team_id, user_id) VALUES
				('ia_team', 'ia_lead'), ('ii_team', 'ii_lead'), ('ad_team', 'ad_lead')`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cleanup() })

	It("counts only active + enabled teams as opted in or not started, and lists no inactive team", func() {
		w := request(http.MethodGet, "/api/v1/admin/survey-completion", "")
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

		var resp dto.SurveyCompletionResponse
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())

		Expect(resp.OptedIn).To(Equal(1), "inactive + enabled team must not be opted in")
		Expect(resp.NotStarted).To(Equal(1), "inactive + enabled team must not be pending")
		Expect(resp.OptedOut).To(Equal(1), "active + disabled team stays opted out")
		Expect(resp.TotalTeams).To(Equal(2), "inactive team is not part of the dashboard")
		Expect(collectTeamIDs(resp.Groups)).To(ConsistOf("ia_team", "ad_team"))
	})

	It("reminds only the active + enabled team's lead", func() {
		w := request(http.MethodPost, "/api/v1/admin/survey-completion/reminders", `{"scope":"org"}`)
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

		var resp dto.SurveyReminderResponse
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Sent).To(Equal(1))

		Expect(mailer.SentHTMLEmails).To(HaveLen(1))
		Expect(mailer.SentHTMLEmails[0].To).To(Equal("ia_lead@test.com"))
	})

	It("does not remind an inactive team directly", func() {
		w := request(http.MethodPost, "/api/v1/admin/survey-completion/reminders", `{"scope":"team","teamId":"ii_team"}`)
		Expect(w.Code).To(Equal(http.StatusNotFound), w.Body.String())
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	It("does not modify the inactive team's row", func() {
		request(http.MethodGet, "/api/v1/admin/survey-completion", "")

		var enabled, active bool
		Expect(db.QueryRow(`SELECT health_check_enabled, is_active FROM teams WHERE id = 'ii_team'`).Scan(&enabled, &active)).To(Succeed())
		Expect(enabled).To(BeTrue())
		Expect(active).To(BeFalse())
	})
})
