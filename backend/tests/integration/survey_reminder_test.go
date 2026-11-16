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
	"github.com/agopalakrishnan/teams360/backend/infrastructure/email"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Survey Completion Reminders", func() {
	const remindersPath = "/api/v1/admin/survey-completion/reminders"

	var (
		db          *sql.DB
		cleanup     func()
		withMail    *gin.Engine
		withoutMail *gin.Engine
		// build wires an admin router with the given reminder options.
		build       func(opts ...v1.AdminOption) *gin.Engine
		mailer      *testhelpers.MockEmailService
		adminToken  string
		memberToken string
	)

	post := func(router *gin.Engine, token, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodPost, remindersPath, bytes.NewBufferString(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService := services.NewJWTService()
		adminPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = adminPair.AccessToken
		memberPair, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = memberPair.AccessToken

		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		healthCheckRepo := postgres.NewHealthCheckRepository(db)

		build = func(opts ...v1.AdminOption) *gin.Engine {
			r := gin.New()
			v1.SetupAdminRoutes(r, orgRepo, userRepo, teamRepo, healthCheckRepo, jwtService, opts...)
			return r
		}
		mailer = testhelpers.NewMockEmailService()
		withMail = build(v1.WithReminderSender(mailer))
		withoutMail = build()

		// A pod with a team lead and no supervisor lands in "Other"; with no
		// submitted surveys it is "not started", so it is remindable.
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			VALUES ('rem_lead', 'remlead', 'remlead@test.com', 'Rem Lead', 'level-4', 'unused')
		`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO teams (id, name, team_lead_id) VALUES ('rem_team', 'Reminder Team', 'rem_lead')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('rem_team', 'rem_lead')`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		db.Exec("DELETE FROM team_members WHERE team_id = 'rem_team'")
		db.Exec("DELETE FROM teams WHERE id = 'rem_team'")
		db.Exec("DELETE FROM users WHERE id = 'rem_lead'")
		cleanup()
	})

	It("emails the team lead of a lagging pod and reports who it reached", func() {
		w := post(withMail, adminToken, `{"scope":"team","teamId":"rem_team"}`)
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

		var resp dto.SurveyReminderResponse
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Sent).To(Equal(1))
		Expect(resp.SentTo).To(ConsistOf("Rem Lead"))

		Expect(mailer.SentHTMLEmails).To(HaveLen(1))
		Expect(mailer.SentHTMLEmails[0].To).To(Equal("remlead@test.com"))
		Expect(mailer.SentHTMLEmails[0].Subject).To(ContainSubstring("Pod Health Check Survey Reminder"))
		Expect(mailer.SentHTMLEmails[0].Body).To(ContainSubstring("Reminder Team"))
	})

	It("answers 503 and sends nothing when email is not configured", func() {
		w := post(withoutMail, adminToken, `{"scope":"team","teamId":"rem_team"}`)

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable), w.Body.String())
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	It("answers 404 for a pod that does not exist", func() {
		w := post(withMail, adminToken, `{"scope":"team","teamId":"no-such-team"}`)

		Expect(w.Code).To(Equal(http.StatusNotFound), w.Body.String())
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	It("answers 400 for an unknown scope, a missing target, and a malformed body", func() {
		for _, body := range []string{`{"scope":"everyone"}`, `{"scope":"team"}`, `{"scope":"person"}`, `not json`} {
			Expect(post(withMail, adminToken, body).Code).To(Equal(http.StatusBadRequest), body)
		}
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	It("answers 400 for an assessment period that does not exist", func() {
		w := post(withMail, adminToken, `{"scope":"team","teamId":"rem_team","assessmentPeriod":"1999 - 1st Half"}`)

		Expect(w.Code).To(Equal(http.StatusBadRequest), w.Body.String())
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	It("rejects a non-admin and an unauthenticated caller", func() {
		Expect(post(withMail, memberToken, `{"scope":"org"}`).Code).To(Equal(http.StatusForbidden))
		Expect(post(withMail, "", `{"scope":"org"}`).Code).To(Equal(http.StatusUnauthorized))
		Expect(mailer.SentHTMLEmails).To(BeEmpty())
	})

	Describe("keeping test runs away from real inboxes", func() {
		It("in log mode sends nothing and says so", func() {
			router := build(v1.WithReminderSender(email.LogSender{}))

			w := post(router, adminToken, `{"scope":"team","teamId":"rem_team"}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var resp dto.SurveyReminderResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp.Mode).To(Equal("log"))
			Expect(resp.Sent).To(BeZero())
			Expect(resp.Logged).To(Equal(1))
			Expect(resp.LoggedFor).To(ConsistOf("Rem Lead"))
		})

		It("with a test pod set, still reaches that pod's own team lead", func() {
			router := build(v1.WithReminderSender(mailer), v1.WithReminderTestPod("reminder team"))

			w := post(router, adminToken, `{"scope":"org"}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var resp dto.SurveyReminderResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp.TestPod).To(Equal("reminder team"))
			Expect(mailer.SentHTMLEmails).To(HaveLen(1), "an org-wide reminder must be cut down to the test pod")
			Expect(mailer.SentHTMLEmails[0].To).To(Equal("remlead@test.com"))
		})

		It("with a test pod that does not exist, sends nothing", func() {
			router := build(v1.WithReminderSender(mailer), v1.WithReminderTestPod("No Such Pod"))

			w := post(router, adminToken, `{"scope":"org"}`)

			Expect(w.Code).To(Equal(http.StatusUnprocessableEntity), w.Body.String())
			Expect(mailer.SentHTMLEmails).To(BeEmpty(), "an unknown test pod must fail closed")
		})
	})
})
