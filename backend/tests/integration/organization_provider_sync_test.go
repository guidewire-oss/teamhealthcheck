package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// providerAPIToken is the plaintext token the fake provider expects, read
// from DATA_PROVIDER_API_TOKEN exactly as the real client would.
const providerAPIToken = "dataprovider-integration-test-token"

// snapshotWithLevels exercises the shapes the real provider payload contains:
// importable people, an exec on a level THC does not configure, a blank-level
// test account, an explicit healthCheckEnabled=false, and a lead who is a member.
const snapshotWithLevels = `{
  "contractVersion": "1.0",
  "generatedAt": "2026-08-31T05:04:48.240Z",
  "teams": [
    {"id": "sync-team-1", "name": "Alcatraz", "healthCheckEnabled": false, "teamLeadId": "sync-user-1"},
    {"id": "sync-team-2", "name": "Alleppey", "healthCheckEnabled": true}
  ],
  "users": [
    {"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"},
    {"id": "sync-user-2", "username": "syncbob", "displayName": "Bob", "email": "syncbob@test.com", "hierarchyLevelId": "level-5", "reportsToId": "sync-user-1"},
    {"id": "sync-exec", "username": "syncceo", "displayName": "Chief", "email": "syncceo@test.com", "hierarchyLevelId": "level-0"},
    {"id": "sync-e2e", "username": "synce2e", "displayName": "E2E", "email": "synce2e@test.com", "hierarchyLevelId": ""}
  ],
  "memberships": [
    {"userId": "sync-user-1", "teamId": "sync-team-1"},
    {"userId": "sync-user-2", "teamId": "sync-team-1"},
    {"userId": "sync-user-2", "teamId": "sync-team-2"}
  ]
}`

var _ = Describe("Integration: Organization Provider Sync", func() {
	var (
		db             *sql.DB
		router         *gin.Engine
		cleanup        func()
		adminToken     string
		memberToken    string
		providerServer *httptest.Server

		// providerBody and providerStatus let each spec shape the upstream reply.
		providerBody   string
		providerStatus int
		// providerGate, when non-nil, blocks the provider until closed.
		providerGate chan struct{}
		// providerHit signals that the provider received a request.
		providerHit chan struct{}
		// gotAPIKey/gotPath record what the last request to the fake provider
		// actually sent, so tests can assert on the real outbound request shape.
		gotAPIKey string
		gotPath   string
	)

	doSync := func(token string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/organization-provider/sync", nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// doSyncWithBody posts the optional request body the override travels in.
	doSyncWithBody := func(token, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/organization-provider/sync", strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// doOverrideSync mirrors the real admin flow: it first probes for the
	// current hold and, when one exists, echoes that hold's own counts back
	// as the confirmation -- exactly as a client would after reading the held
	// sync's response. It never hardcodes the counts, so it keeps matching
	// whatever each test's own fixture setup actually trips the guard with.
	// If the probe itself does not come back as a hold (a non-admin token
	// rejected before the guard ever runs, or a provider/validation failure
	// that would fail identically on the real attempt too), it falls through
	// with zeroed confirmation and lets the real request fail the same way.
	doOverrideSync := func(token string) *httptest.ResponseRecorder {
		confirmed := map[string]int{"usersExisting": 0, "usersDeleting": 0, "teamsExisting": 0, "teamsDeleting": 0}
		if held := doSync(token); held.Code == http.StatusConflict {
			var holdBody dto.MassDeletionHoldResponseDTO
			Expect(json.Unmarshal(held.Body.Bytes(), &holdBody)).To(Succeed())
			if holdBody.MassDeletion != nil {
				confirmed["usersExisting"] = holdBody.MassDeletion.Users.Existing
				confirmed["usersDeleting"] = holdBody.MassDeletion.Users.Deleting
				confirmed["teamsExisting"] = holdBody.MassDeletion.Teams.Existing
				confirmed["teamsDeleting"] = holdBody.MassDeletion.Teams.Deleting
			}
		}

		body, err := json.Marshal(map[string]any{
			"overrideMassDeletion":  true,
			"confirmedMassDeletion": confirmed,
		})
		Expect(err).NotTo(HaveOccurred())
		return doSyncWithBody(token, string(body))
	}

	getSettings := func(token string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, "/api/v1/admin/settings/organization-provider", nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	countRows := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-integration-tests")
		// Neutral by default: this suite's fixture populations are tiny (2-4
		// users), so even one legitimate, correctly-scoped deletion can exceed
		// a realistic percentage threshold. Only the dedicated mass-deletion
		// guard test below overrides this to a strict value -- every other
		// test is exercising something else and must not incidentally trip it.
		os.Setenv(services.EnvMaxDeletePercent, "100")
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		providerBody = snapshotWithLevels
		providerStatus = http.StatusOK
		providerGate = nil
		providerHit = make(chan struct{}, 10)

		providerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			providerHit <- struct{}{}
			gotAPIKey = r.Header.Get("x-api-key")
			gotPath = r.URL.Path
			if gotAPIKey != providerAPIToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if providerGate != nil {
				<-providerGate
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(providerStatus)
			_, _ = w.Write([]byte(providerBody))
		}))
		os.Setenv(dataprovider.EnvBaseURL, providerServer.URL)
		os.Setenv(dataprovider.EnvAPIToken, providerAPIToken)

		jwtService := services.NewJWTService()
		adminPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = adminPair.AccessToken

		memberPair, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = memberPair.AccessToken

		providerRepo := postgres.NewOrganizationProviderRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)

		dataProviderCfg, err := dataprovider.LoadConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(dataProviderCfg).NotTo(BeNil())
		dataProviderClient, err := dataprovider.NewClient(dataProviderCfg)
		Expect(err).NotTo(HaveOccurred())

		syncService := services.NewOrganizationSyncService(providerRepo, dataProviderClient, userRepo, teamRepo)

		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, syncService, postgres.NewOrganizationRepository(db), jwtService)
	})

	AfterEach(func() {
		providerServer.Close()
		cleanup()
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv(services.EnvMaxDeletePercent)
		os.Unsetenv(dataprovider.EnvBaseURL)
		os.Unsetenv(dataprovider.EnvAPIToken)
	})

	Describe("Outbound request to the data provider", func() {
		It("calls GET /org-snapshot with only the x-api-key header", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			Expect(gotPath).To(Equal("/org-snapshot"))
			Expect(gotAPIKey).To(Equal(providerAPIToken))
		})
	})

	Describe("POST /api/v1/admin/organization-provider/sync", func() {
		It("should import users, teams and memberships for an admin", func() {
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())

			Expect(result.Status).To(Equal("completed"))
			Expect(result.UsersSynced).To(Equal(2), "only the two users on configured levels import")
			Expect(result.TeamsSynced).To(Equal(2))
			Expect(result.MembershipsSynced).To(Equal(3))
			Expect(result.StartedAt).NotTo(BeZero())
			Expect(result.CompletedAt).NotTo(BeZero())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(2))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id LIKE 'sync-team-%'`)).To(Equal(2))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id LIKE 'sync-team-%'`)).To(Equal(3))
		})

		It("should set the manager link once every user exists", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			var reportsTo sql.NullString
			Expect(db.QueryRow(`SELECT reports_to FROM users WHERE id = 'sync-user-2'`).Scan(&reportsTo)).To(Succeed())
			Expect(reportsTo.Valid).To(BeTrue())
			Expect(reportsTo.String).To(Equal("sync-user-1"))
		})

		It("should skip users whose hierarchy level is not configured and report them, not as a completeness error", func() {
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())

			Expect(result.UsersSkipped).To(Equal(2))
			Expect(result.SkippedUsers).To(HaveLen(2))

			reasons := map[string]string{}
			for _, s := range result.SkippedUsers {
				reasons[s.UserID] = s.Reason
			}
			Expect(reasons["sync-exec"]).To(Equal("unknown_hierarchy_level"))
			Expect(reasons["sync-e2e"]).To(Equal("missing_hierarchy_level"))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id IN ('sync-exec','sync-e2e')`)).To(Equal(0))
		})

		It("should map healthCheckEnabled onto teams, applying both true and false", func() {
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())

			var team1, team2 bool
			Expect(db.QueryRow(`SELECT health_check_enabled FROM teams WHERE id = 'sync-team-1'`).Scan(&team1)).To(Succeed())
			Expect(db.QueryRow(`SELECT health_check_enabled FROM teams WHERE id = 'sync-team-2'`).Scan(&team2)).To(Succeed())
			Expect(team1).To(BeFalse(), "healthCheckEnabled=false must apply, not be treated as missing")
			Expect(team2).To(BeTrue(), "healthCheckEnabled=true should apply")
			Expect(result.HealthChecksDisabled).To(BeZero(), "a newly created team was never participating, so nothing was switched off")
		})

		It("should report how many participating teams it switches off", func() {
			_, err := db.Exec(`
				INSERT INTO teams (id, name, health_check_enabled) VALUES ('sync-team-1', 'Alcatraz', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.HealthChecksDisabled).To(Equal(1))
		})

		It("should not count a protected team's healthCheckEnabled flag as a transition", func() {
			_, err := db.Exec(`
				INSERT INTO teams (id, name, health_check_enabled) VALUES ('sync-team-1', 'Alcatraz', true)
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO teams (id, name, health_check_enabled) VALUES ('team-phoenix', 'Phoenix', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [
					{"id": "sync-team-1", "name": "Alcatraz", "healthCheckEnabled": false},
					{"id": "team-phoenix", "name": "Phoenix", "healthCheckEnabled": false}
				],
				"users": [{"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"}],
				"memberships": []
			}`

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.HealthChecksDisabled).To(Equal(1), "only the non-protected team's disable must be counted -- upsertSnapshotTeams never writes team-phoenix")

			var stillEnabled bool
			Expect(db.QueryRow(`SELECT health_check_enabled FROM teams WHERE id = 'team-phoenix'`).Scan(&stillEnabled)).To(Succeed())
			Expect(stillEnabled).To(BeTrue(), "a protected team's health_check_enabled must never be written by a sync")
		})

		It("should not rebuild a protected team's supervisor chain", func() {
			// sync-stale-supervisor has no reports_to, so if rederiveSupervisorChains
			// ran for team-phoenix it would derive an empty chain and delete this row
			// (UpdateSupervisorChain replaces a team's chain wholesale).
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-stale-supervisor', 'stalesup', 'stalesup@test.com', 'Stale Supervisor', 'level-3', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id) VALUES ('team-phoenix', 'Phoenix', 'sync-stale-supervisor')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position) VALUES ('team-phoenix', 'sync-stale-supervisor', 'level-3', 1)
			`)
			Expect(err).NotTo(HaveOccurred())

			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [
					{"id": "sync-team-1", "name": "Alcatraz"},
					{"id": "team-phoenix", "name": "Phoenix"}
				],
				"users": [{"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"}],
				"memberships": []
			}`

			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			Expect(countRows(`
				SELECT COUNT(*) FROM team_supervisors WHERE team_id = 'team-phoenix' AND user_id = 'sync-stale-supervisor'
			`)).To(Equal(1), "a protected team's supervisor chain must never be rebuilt by a sync")
		})

		It("should preserve the existing team-lead value when the provider omits teamLeadId", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-existing-lead', 'existinglead', 'existinglead@test.com', 'Existing Lead', 'level-4', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id, health_check_enabled) VALUES ('sync-team-3', 'Andalusia', 'sync-existing-lead', false)
			`)
			Expect(err).NotTo(HaveOccurred())

			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [{"id": "sync-team-3", "name": "Andalusia Renamed"}],
				"users": [{"id": "sync-existing-lead", "username": "existinglead", "displayName": "Existing Lead", "email": "existinglead@test.com", "hierarchyLevelId": "level-4"}],
				"memberships": []
			}`

			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			var name string
			var teamLead sql.NullString
			var enabled bool
			Expect(db.QueryRow(`SELECT name, team_lead_id, health_check_enabled FROM teams WHERE id = 'sync-team-3'`).Scan(&name, &teamLead, &enabled)).To(Succeed())
			Expect(name).To(Equal("Andalusia Renamed"), "other fields still update")
			Expect(teamLead.String).To(Equal("sync-existing-lead"), "an omitted teamLeadId must not clear the existing lead")
			Expect(enabled).To(BeFalse(), "an omitted healthCheckEnabled must not be overwritten")
		})

		It("should be idempotent when run twice", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			first := countRows(`SELECT COUNT(*) FROM team_members WHERE team_id LIKE 'sync-team-%'`)

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.UsersSynced).To(Equal(2))
			Expect(result.MembershipsRemoved).To(BeZero(), "a repeat sync should not churn membership rows")
			Expect(result.UsersDeactivated).To(BeZero(), "nothing outside the snapshot is missing on a repeat run")
			Expect(result.TeamsDeactivated).To(BeZero())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(2))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id LIKE 'sync-team-%'`)).To(Equal(first))
		})

		Describe("an existing SSO user stored under a different id than the provider sends", func() {
			insertUser := func(id, username, email, authType string) {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
					VALUES ($1, $2, $3, 'Legacy', 'level-3', 'keep-hash', $4)
				`, id, username, email, authType)
				Expect(err).NotTo(HaveOccurred())
			}

			// expectNothingRekeyed runs a sync that must be refused and checks the
			// given legacy rows are untouched and no row was created for sync-user-1.
			expectNothingRekeyed := func(legacyIDs ...string) {
				Expect(doSync(adminToken).Code).NotTo(Equal(http.StatusOK))
				for _, id := range legacyIDs {
					Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = $1`, id)).To(Equal(1))
				}
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-user-1'`)).To(Equal(0))
			}

			It("should re-key the row to the provider id by email without deleting anything attached to it", func() {
				// syncalice is stored with her username as the id; the snapshot sends sync-user-1.
				insertUser("syncalice", "syncalice", "SyncAlice@test.com", "sso")
				_, err := db.Exec(`INSERT INTO teams (id, name, team_lead_id) VALUES ('sync-team-1', 'Alcatraz', 'syncalice')`)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'syncalice')`)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec(`
					INSERT INTO action_items (id, team_id, created_by, assigned_to, title)
					VALUES ('sync-rekey-action', 'sync-team-1', 'syncalice', 'syncalice', 'Follow up')
				`)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('sync-team-1', 'syncalice', 'level-3', 1)
				`)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES ('sync-rekey-session', 'sync-team-1', 'syncalice', '2024-01-01', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				w := doSync(adminToken)
				Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

				var result services.SyncResult
				Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
				Expect(result.UsersDeactivated).To(BeZero(), "a re-keyed user is the same person, not a departure")

				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'syncalice'`)).To(Equal(0))
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE email ILIKE 'syncalice@test.com'`)).To(Equal(1), "no duplicate row")
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-user-1' AND password_hash = 'keep-hash' AND auth_type = 'sso'`)).To(Equal(1),
					"the stored credentials and auth type must survive the re-key")

				Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-user-1'`)).To(Equal(1))
				Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-team-1' AND team_lead_id = 'sync-user-1'`)).To(Equal(1))
				Expect(countRows(`SELECT COUNT(*) FROM action_items WHERE id = 'sync-rekey-action' AND created_by = 'sync-user-1' AND assigned_to = 'sync-user-1'`)).To(Equal(1),
					"the action item must follow the user, not cascade away")
				// The sync service rebuilds supervisor chains from the team lead's reports_to
				// chain afterwards, and this lead has no manager, so the row is expected to
				// be gone. What matters is that nothing is left under the old id.
				Expect(countRows(`SELECT COUNT(*) FROM team_supervisors WHERE user_id = 'syncalice'`)).To(Equal(0),
					"no supervisor link may be left on the old id")
				Expect(countRows(`SELECT COUNT(*) FROM health_check_sessions WHERE id = 'sync-rekey-session' AND user_id = 'sync-user-1'`)).To(Equal(1),
					"assessment history must follow the user")
			})

			It("should fall back to the username when the email differs", func() {
				insertUser("syncalice", "syncalice", "old-alice@test.com", "sso")

				Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'syncalice'`)).To(Equal(0))
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-user-1' AND email = 'syncalice@test.com'`)).To(Equal(1))
			})

			It("should not re-key a local-login user, leaving the existing conflict error", func() {
				insertUser("syncalice", "syncalice", "syncalice@test.com", "local")

				expectNothingRekeyed("syncalice")
			})

			It("should re-key an SSO administrator by email and keep their administrator level", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
					VALUES ('alice-admin', 'syncalice', 'syncalice@test.com', 'Alice Admin', 'level-admin', '', 'sso')
				`)
				Expect(err).NotTo(HaveOccurred())

				w := doSync(adminToken)
				Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'alice-admin'`)).To(Equal(0))
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-user-1' AND hierarchy_level_id = 'level-admin'`)).To(Equal(1),
					"the provider must not be able to change an administrator's level")
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE email ILIKE 'syncalice@test.com'`)).To(Equal(1))
			})

			It("should never re-key a fixture account, even when its email matches the provider", func() {
				insertUser("e2e_demo", "syncalice", "syncalice@test.com", "sso")

				expectNothingRekeyed("e2e_demo")
			})

			It("should not take over a user the provider still sends under their own id", func() {
				// sync-user-2 is a live provider user, but still holds the username the
				// provider has since given to sync-user-1. Matching on that username must
				// not move sync-user-2's history onto sync-user-1.
				insertUser("sync-user-2", "syncalice", "old-bob@test.com", "sso")
				_, err := db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES ('sync-takeover-session', 'sync-team-1', 'sync-user-2', '2024-01-01', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				doSync(adminToken)

				Expect(countRows(`SELECT COUNT(*) FROM health_check_sessions WHERE id = 'sync-takeover-session' AND user_id = 'sync-user-2'`)).To(Equal(1),
					"a live user's history must stay with them")
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-user-2'`)).To(Equal(1))
			})

			It("should repoint every column that has a foreign key to users(id)", func() {
				// Re-keying deletes the old row, so a missed foreign key would cascade
				// that data away. health_check_sessions.user_id is repointed too but has
				// no foreign key, so it is not checked here.
				rows, err := db.Query(`
					SELECT c.conrelid::regclass::text, a.attname
					FROM pg_constraint c
					JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
					WHERE c.confrelid = 'users'::regclass AND c.contype = 'f'
				`)
				Expect(err).NotTo(HaveOccurred())
				defer rows.Close()

				covered := postgres.UserIDReferenceColumns()
				for rows.Next() {
					var table, column string
					Expect(rows.Scan(&table, &column)).To(Succeed())
					Expect(covered).To(ContainElement(table+"."+column),
						"add %s.%s to userIDReferences so re-keying repoints it", table, column)
				}
				Expect(rows.Err()).NotTo(HaveOccurred())
			})

			It("should keep every repointed column wide enough to hold any users.id", func() {
				var idWidth int
				Expect(db.QueryRow(`
					SELECT character_maximum_length FROM information_schema.columns
					WHERE table_name = 'users' AND column_name = 'id'
				`).Scan(&idWidth)).To(Succeed())

				for _, ref := range postgres.UserIDReferenceColumns() {
					table, column, _ := strings.Cut(ref, ".")
					var width int
					Expect(db.QueryRow(`
						SELECT character_maximum_length FROM information_schema.columns
						WHERE table_name = $1 AND column_name = $2
					`, table, column).Scan(&width)).To(Succeed(), ref)
					Expect(width).To(BeNumerically(">=", idWidth),
						"%s is narrower than users.id, so re-keying to a long provider id would fail", ref)
				}
			})

			It("should refuse to guess when the email and username match two different rows", func() {
				insertUser("alice-by-email", "someoneelse", "syncalice@test.com", "sso")
				insertUser("alice-by-username", "syncalice", "other@test.com", "sso")

				expectNothingRekeyed("alice-by-email", "alice-by-username")
			})
		})

		It("should make the provider authoritative for membership per team while protecting skipped users", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			// A person the provider no longer lists on the team...
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-stale', 'syncstale', 'syncstale@test.com', 'Stale', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'sync-stale')`)
			Expect(err).NotTo(HaveOccurred())

			// ...and a skipped user who already belongs to it.
			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-exec', 'syncceo', 'syncceo@test.com', 'Chief', 'level-1', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'sync-exec')`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.MembershipsRemoved).To(Equal(1))

			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-stale' AND is_active`,
			)).To(Equal(0), "a member the provider dropped should no longer be active")
			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-stale' AND is_active = false`,
			)).To(Equal(1), "...but the membership row is kept as history")

			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-exec'`,
			)).To(Equal(1), "a skipped user must keep the membership THC already had")

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-exec'`)).To(Equal(1),
				"a skipped user's own account must not be hard-deleted either -- being unimportable this sync is not evidence the provider removed them")
		})

		It("should clear all memberships for a team the provider returns with zero of them", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1'`)).To(BeNumerically(">", 0))

			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T06:00:00.000Z",
				"teams": [
					{"id": "sync-team-1", "name": "Alcatraz", "healthCheckEnabled": false},
					{"id": "sync-team-2", "name": "Alleppey", "healthCheckEnabled": true}
				],
				"users": [
					{"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"},
					{"id": "sync-user-2", "username": "syncbob", "displayName": "Bob", "email": "syncbob@test.com", "hierarchyLevelId": "level-5"}
				],
				"memberships": []
			}`

			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id IN ('sync-team-1','sync-team-2') AND is_active`)).To(Equal(0), "an explicit empty membership list must end every active membership")
		})

		It("keeps Himani's Hopefarm history when she moves to SRE, and assigns future surveys only through SRE", func() {
			snapshot := func(generatedAt string, himaniTeam string) string {
				return `{"contractVersion":"1.0","generatedAt":"` + generatedAt + `",
					"teams":[{"id":"sync-hopefarm","name":"Hopefarm"},{"id":"sync-sre","name":"SRE"}],
					"users":[
						{"id":"sync-himani","username":"synchimani","displayName":"Himani","email":"synchimani@test.com","hierarchyLevelId":"level-5"},
						{"id":"sync-other","username":"syncother","displayName":"Other","email":"syncother@test.com","hierarchyLevelId":"level-5"}],
					"memberships":[{"userId":"sync-himani","teamId":"` + himaniTeam + `"},{"userId":"sync-other","teamId":"sync-hopefarm"}]}`
			}

			providerBody = snapshot("2026-01-10T00:00:00Z", "sync-hopefarm")
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			// Himani completes a Q1 Hopefarm survey, with a response and an action item.
			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
				VALUES ('sync-q1-session', 'sync-hopefarm', 'sync-himani', '2026-02-01', '2026 - 1st Half', true)`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO health_check_responses (session_id, dimension_id, score, trend) VALUES ('sync-q1-session','mission',3,'stable')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO action_items (id, team_id, created_by, title) VALUES ('sync-q1-action','sync-hopefarm','sync-himani','Q1 follow-up')`)
			Expect(err).NotTo(HaveOccurred())

			// Later the provider says she is in SRE (and no longer in Hopefarm).
			providerBody = snapshot("2026-04-10T00:00:00Z", "sync-sre")
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE user_id='sync-himani' AND team_id='sync-hopefarm' AND is_active = false AND deactivated_at IS NOT NULL`)).
				To(Equal(1), "the Hopefarm membership is kept as inactive history, not deleted")
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE user_id='sync-himani' AND team_id='sync-sre' AND is_active`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id='sync-himani' AND is_active`)).To(Equal(1), "she still has an active membership, so she can sign in")

			Expect(countRows(`SELECT COUNT(*) FROM health_check_sessions WHERE id='sync-q1-session' AND team_id='sync-hopefarm' AND user_id='sync-himani'`)).
				To(Equal(1), "the Q1 survey stays under Hopefarm")
			Expect(countRows(`SELECT COUNT(*) FROM health_check_responses WHERE session_id='sync-q1-session'`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM action_items WHERE id='sync-q1-action' AND team_id='sync-hopefarm'`)).To(Equal(1))

			teamRepo := postgres.NewTeamRepository(db)
			eligible, err := teamRepo.FindEligibleMemberIDs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(eligible["sync-sre"]).To(ContainElement("sync-himani"), "future surveys go to SRE")
			Expect(eligible["sync-hopefarm"]).NotTo(ContainElement("sync-himani"), "...and not to Hopefarm")

			// An older snapshot arriving late must not move her back.
			providerBody = snapshot("2026-01-10T00:00:00Z", "sync-hopefarm")
			Expect(doSync(adminToken).Code).NotTo(Equal(http.StatusOK))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE user_id='sync-himani' AND team_id='sync-hopefarm' AND is_active`)).To(Equal(0))
		})

		It("reports only real changes: an identical re-sync is 0 added, 0 updated, all unchanged, 0 membership changes", func() {
			syncResult := func() services.SyncResult {
				w := doSync(adminToken)
				Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
				var r services.SyncResult
				Expect(json.Unmarshal(w.Body.Bytes(), &r)).To(Succeed())
				return r
			}

			first := syncResult()
			Expect(first.InsertedUsers).To(BeNumerically(">", 0), "the first sync adds the fixture people")
			Expect(first.InsertedTeams).To(BeNumerically(">", 0))

			var updatedAtBefore string
			Expect(db.QueryRow(`SELECT updated_at::text FROM users WHERE id = 'sync-user-1'`).Scan(&updatedAtBefore)).To(Succeed())

			second := syncResult()
			Expect(second.InsertedUsers).To(BeZero())
			Expect(second.UpdatedUsers).To(BeZero(), "reading a record is not updating it")
			Expect(second.UnchangedUsers).To(Equal(first.InsertedUsers))
			Expect(second.InsertedTeams).To(BeZero())
			Expect(second.UpdatedTeams).To(BeZero())
			Expect(second.UnchangedTeams).To(Equal(first.InsertedTeams))
			Expect(second.MembershipChanges).To(BeZero())
			Expect(second.UsersDeactivated).To(BeZero())
			Expect(second.TeamsDeactivated).To(BeZero())

			var updatedAtAfter string
			Expect(db.QueryRow(`SELECT updated_at::text FROM users WHERE id = 'sync-user-1'`).Scan(&updatedAtAfter)).To(Succeed())
			Expect(updatedAtAfter).To(Equal(updatedAtBefore), "an unchanged record must not even have its timestamp touched")
		})

		It("counts a record as updated only when a business field really changed, and ignores case and whitespace noise", func() {
			firstResp := doSync(adminToken)
			Expect(firstResp.Code).To(Equal(http.StatusOK))
			var initial services.SyncResult
			Expect(json.Unmarshal(firstResp.Body.Bytes(), &initial)).To(Succeed())
			// Some fixture users are skipped for an unknown hierarchy level, so the
			// imported count -- not the payload size -- is what "unchanged" is out of.
			imported := initial.InsertedUsers
			Expect(imported).To(BeNumerically(">", 1))

			var snap map[string]any
			Expect(json.Unmarshal([]byte(providerBody), &snap)).To(Succeed())
			users := snap["users"].([]any)
			first := users[0].(map[string]any)
			first["email"] = strings.ToUpper(first["email"].(string))          // same address, different case
			first["displayName"] = "  " + first["displayName"].(string) + "  " // same name, extra spaces
			b, _ := json.Marshal(snap)
			providerBody = string(b)

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			var noise services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &noise)).To(Succeed())
			Expect(noise.UpdatedUsers).To(BeZero(), "case/whitespace-only differences are not a change")

			first["displayName"] = "Renamed Person"
			b, _ = json.Marshal(snap)
			providerBody = string(b)

			w = doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			var changed services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &changed)).To(Succeed())
			Expect(changed.UpdatedUsers).To(Equal(1))
			Expect(changed.InsertedUsers).To(BeZero())
			Expect(changed.UnchangedUsers).To(Equal(imported-1), "everyone imported except the one renamed person is unchanged")
		})

		Describe("team status follows the provider snapshot, not member or lead changes", func() {
			// teamSnapshot builds a complete snapshot with two teams (Hopefarm, SRE)
			// and two people. lead is Hopefarm's team lead id ("" omits it);
			// hopefarmMembers/sreMembers are the member ids of each team; includeSRE
			// controls whether SRE appears in the snapshot at all.
			teamSnapshot := func(generatedAt, lead string, hopefarmMembers, sreMembers []string, includeSRE bool) string {
				hopefarm := `{"id":"ts-hopefarm","name":"Hopefarm"`
				if lead != "" {
					hopefarm += `,"teamLeadId":"` + lead + `"`
				}
				hopefarm += `}`
				teams := hopefarm
				memberships := ""
				add := func(teamID string, ids []string) {
					for _, id := range ids {
						if memberships != "" {
							memberships += ","
						}
						memberships += `{"userId":"` + id + `","teamId":"` + teamID + `"}`
					}
				}
				add("ts-hopefarm", hopefarmMembers)
				if includeSRE {
					teams += `,{"id":"ts-sre","name":"SRE"}`
					add("ts-sre", sreMembers)
				}
				return `{"contractVersion":"1.0","generatedAt":"` + generatedAt + `",
					"teams":[` + teams + `],
					"users":[
						{"id":"ts-a","username":"tsalpha","displayName":"Alpha","email":"tsalpha@test.com","hierarchyLevelId":"level-5"},
						{"id":"ts-b","username":"tsbeta","displayName":"Beta","email":"tsbeta@test.com","hierarchyLevelId":"level-5"}],
					"memberships":[` + memberships + `]}`
			}
			teamActive := func(id string) int {
				return countRows(`SELECT COUNT(*) FROM teams WHERE id = $1 AND is_active AND deactivated_at IS NULL`, id)
			}
			runSync := func() services.SyncResult {
				w := doSync(adminToken)
				Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
				var r services.SyncResult
				Expect(json.Unmarshal(w.Body.Bytes(), &r)).To(Succeed())
				return r
			}

			It("keeps a team active when only its team lead and members change", func() {
				providerBody = teamSnapshot("2026-01-10T00:00:00Z", "", []string{"ts-a", "ts-b"}, nil, true)
				runSync()
				Expect(teamActive("ts-hopefarm")).To(Equal(1))

				// Lead appears, one member leaves, the other joins SRE.
				providerBody = teamSnapshot("2026-02-10T00:00:00Z", "ts-a", []string{"ts-a"}, []string{"ts-b"}, true)
				second := runSync()

				Expect(second.TeamsDeactivated).To(BeZero(), "a lead or membership change is not a team removal")
				Expect(teamActive("ts-hopefarm")).To(Equal(1))
				Expect(teamActive("ts-sre")).To(Equal(1))
				Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id='ts-hopefarm' AND team_lead_id='ts-a'`)).To(Equal(1), "the lead change itself is applied")
				Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id='ts-hopefarm' AND user_id='ts-b' AND is_active = false`)).To(Equal(1),
					"the member who left keeps an inactive membership")
			})

			It("keeps a team active when its members are emptied but the provider still reports the team", func() {
				providerBody = teamSnapshot("2026-01-10T00:00:00Z", "", []string{"ts-a", "ts-b"}, nil, true)
				runSync()
				providerBody = teamSnapshot("2026-02-10T00:00:00Z", "", nil, []string{"ts-a", "ts-b"}, true)
				Expect(runSync().TeamsDeactivated).To(BeZero())
				Expect(teamActive("ts-hopefarm")).To(Equal(1), "still reported, so still active, even with no members")
			})

			It("marks a team inactive only when it is missing from a COMPLETE snapshot, and never deletes it", func() {
				providerBody = teamSnapshot("2026-01-10T00:00:00Z", "", []string{"ts-a"}, []string{"ts-b"}, true)
				runSync()

				providerBody = teamSnapshot("2026-02-10T00:00:00Z", "", []string{"ts-a"}, nil, false) // SRE removed
				removed := runSync()

				Expect(removed.TeamsDeactivated).To(Equal(1))
				Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id='ts-sre'`)).To(Equal(1), "never hard-deleted")
				Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id='ts-sre' AND is_active = false AND deactivated_at IS NOT NULL`)).To(Equal(1))
				Expect(teamActive("ts-hopefarm")).To(Equal(1), "teams still in the snapshot are untouched")
			})

			It("reactivates a team the provider reports again", func() {
				providerBody = teamSnapshot("2026-01-10T00:00:00Z", "", []string{"ts-a"}, []string{"ts-b"}, true)
				runSync()
				providerBody = teamSnapshot("2026-02-10T00:00:00Z", "", []string{"ts-a"}, nil, false)
				runSync()
				Expect(teamActive("ts-sre")).To(BeZero())

				providerBody = teamSnapshot("2026-03-10T00:00:00Z", "", []string{"ts-a"}, []string{"ts-b"}, true)
				back := runSync()

				Expect(teamActive("ts-sre")).To(Equal(1), "reported again, so active again")
				Expect(back.UpdatedTeams).To(BeNumerically(">=", 1), "a reactivation is a real change")
			})

			It("leaves a team untouched when it is missing from a PARTIAL snapshot", func() {
				providerBody = teamSnapshot("2026-01-10T00:00:00Z", "", []string{"ts-a"}, []string{"ts-b"}, true)
				runSync()

				var snap map[string]any
				Expect(json.Unmarshal([]byte(teamSnapshot("2026-02-10T00:00:00Z", "", []string{"ts-a"}, nil, false)), &snap)).To(Succeed())
				snap["snapshotType"] = "PARTIAL"
				b, _ := json.Marshal(snap)
				providerBody = string(b)

				Expect(runSync().TeamsDeactivated).To(BeZero())
				Expect(teamActive("ts-sre")).To(Equal(1), "a partial payload says nothing about who is absent")
			})
		})

		It("leaves records absent from a PARTIAL snapshot untouched", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-absent', 'syncabsent', 'syncabsent@test.com', 'Absent', 'level-5', '')`)
			Expect(err).NotTo(HaveOccurred())

			var snap map[string]any
			Expect(json.Unmarshal([]byte(providerBody), &snap)).To(Succeed())
			snap["snapshotType"] = "PARTIAL"
			snap["generatedAt"] = "2099-01-01T00:00:00Z"
			b, _ := json.Marshal(snap)
			providerBody = string(b)

			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id='sync-absent' AND is_active`)).To(Equal(1))
		})

		It("should soft-deactivate a non-protected user missing from the complete snapshot", func() {
			Expect(doSync(adminToken).Code).To(Equal(http.StatusOK))

			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-departed', 'syncdeparted', 'syncdeparted@test.com', 'Departed', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.UsersDeactivated).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-departed'`)).To(Equal(1), "people are never hard-deleted")
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-departed' AND is_active = false`)).To(Equal(1))
		})

		It("should deactivate a non-protected team missing from the complete snapshot and end its memberships, keeping the rows", func() {
			_, err := db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-gone-team', 'Gone Team')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-gone-member', 'syncgonemember', 'syncgonemember@test.com', 'Gone Member', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-gone-team', 'sync-gone-member')`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.TeamsDeactivated).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-gone-team' AND is_active = false`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-gone-team' AND is_active = false`)).To(Equal(1), "membership is kept as inactive history")
		})

		It("should deactivate a user or team that owns an action item and keep the action item", func() {
			// Per the approved design, the provider is authoritative and nothing about
			// owning an action item exempts a record from deletion -- unlike
			// health-check history, action_items has no FK-less escape hatch
			// (action_items.created_by/team_id are NOT NULL with ON DELETE
			// CASCADE; see migrations/000020_create_action_items). This test
			// proves the cascade happens and is surfaced, not silently hidden
			// and not used to block the deletion.
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-owner', 'syncowner', 'syncowner@test.com', 'Owner', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-owned-team', 'Owned Team')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO action_items (id, team_id, created_by, title)
				VALUES ('sync-action-1', 'sync-owned-team', 'sync-owner', 'Follow up')
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.UsersDeactivated).To(BeNumerically(">=", 1))
			Expect(result.TeamsDeactivated).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-owner' AND is_active = false`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-owned-team' AND is_active = false`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM action_items WHERE id = 'sync-action-1'`)).To(Equal(1), "action items survive deactivation")
		})

		It("should never delete or alter health-check history when a user/team is deactivated", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-history-user', 'synchistoryuser', 'synchistoryuser@test.com', 'History User', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-history-team', 'History Team')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
				VALUES ('sync-history-session', 'sync-history-team', 'sync-history-user', '2024-01-01', '2024 - 1st Half', true)
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
				VALUES ('sync-history-session', 'mission', 3, 'stable')
			`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.UsersDeactivated).To(Equal(1))
			Expect(result.TeamsDeactivated).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-history-user' AND is_active = false`)).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-history-team' AND is_active = false`)).To(Equal(1))

			// The session/response rows themselves are untouched -- there is no
			// foreign key from health_check_sessions to users/teams (deferred in
			// migrations/000013_add_security_constraints), so hard-deleting the
			// user/team can never cascade into this data. The rows now reference
			// an id that no longer exists in users/teams -- a pre-existing gap,
			// not something this feature introduces or worsens.
			Expect(countRows(`SELECT COUNT(*) FROM health_check_sessions WHERE id = 'sync-history-session'`)).To(Equal(1), "history must survive even though its owning user/team was deleted")
			Expect(countRows(`SELECT COUNT(*) FROM health_check_responses WHERE session_id = 'sync-history-session'`)).To(Equal(1))
		})

		It("should protect the permanent admin account from deletion and upsert", func() {
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'admin'`)).To(Equal(1), "the permanent admin must survive a sync that never mentions it")
		})

		It("should protect every documented demo/test/E2E fixture id from deletion", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash) VALUES
				('demo', 'demo', 'demo@teams360.demo', 'Demo User', 'level-5', ''),
				('vp', 'vp', 'vp@teams360.demo', 'VP', 'level-1', ''),
				('e2e_demo', 'e2e_demo', 'e2e_demo@teams360.demo', 'E2E Demo', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('team-phoenix', 'Phoenix Squad')`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.UsersDeactivated).To(BeZero(), "protected fixtures are excluded from the deletion scope entirely")

			for _, id := range []string{"demo", "vp", "e2e_demo"} {
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = $1`, id)).To(Equal(1), id+" must survive")
			}
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'team-phoenix'`)).To(Equal(1))
		})

		It("should protect a protected user's membership in a non-protected, provider-managed team", func() {
			// 'demo' is a fixed fixture user, never sent by the provider. Give it a
			// membership in a real, non-protected team that IS in the snapshot
			// (sync-team-1). If protection only covered the users/teams tables
			// and not team_members, this membership would be silently pruned by
			// replaceSnapshotMemberships because 'demo' never appears in
			// sync-team-1's memberships[] entry.
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('demo', 'demo', 'demo@teams360.demo', 'Demo User', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-team-1', 'Alcatraz')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'demo')`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'demo'`)).To(Equal(1))
			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'demo'`,
			)).To(Equal(1), "a protected user's membership must survive reconciliation of a non-protected team too, not only their own users row")
		})

		It("should not insert a membership for a protected user absent locally, without failing the sync", func() {
			// 'demo' is never inserted into users before this sync, and the
			// snapshot incoming claims it as a member of sync-team-1. Before the
			// fix, replaceSnapshotMemberships would try to INSERT INTO
			// team_members for 'demo' even though upsertSnapshotUsers skipped
			// writing its users row, violating the team_members -> users FK.
			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [{"id": "sync-team-1", "name": "Alcatraz"}],
				"users": [
					{"id": "demo", "username": "demo", "displayName": "Demo User", "email": "demo@teams360.demo", "hierarchyLevelId": "level-5"}
				],
				"memberships": [
					{"userId": "demo", "teamId": "sync-team-1"}
				]
			}`

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'demo'`)).To(BeZero(),
				"a protected id must never be created by a sync, even when the provider claims it")
			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'demo'`,
			)).To(BeZero(), "a membership cannot exist for a users row the sync never wrote")
		})

		It("should not touch an existing protected user's membership when the provider also claims it", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('demo', 'demo', 'demo@teams360.demo', 'Demo User', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-team-1', 'Alcatraz')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'demo')`)
			Expect(err).NotTo(HaveOccurred())

			providerBody = `{
				"contractVersion": "1.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [{"id": "sync-team-1", "name": "Alcatraz"}],
				"users": [
					{"id": "demo", "username": "demo", "displayName": "Demo User", "email": "demo@teams360.demo", "hierarchyLevelId": "level-5"}
				],
				"memberships": [
					{"userId": "demo", "teamId": "sync-team-1"}
				]
			}`

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'demo'`)).To(Equal(1))
			Expect(countRows(
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'demo'`,
			)).To(Equal(1), "an existing protected user's membership must be preserved, not pruned or duplicated")
		})

		It("should never delete a user currently holding the level-admin hierarchy level, even if stale candidate data named them", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('sync-promoted-admin', 'promotedadmin', 'promotedadmin@test.com', 'Promoted Admin', 'level-admin', '')
			`)
			Expect(err).NotTo(HaveOccurred())

			// The snapshot never mentions this user, which is exactly the stale
			// candidate case: the user is a deletion candidate by every earlier
			// check, but is now level-admin in the database at delete time.
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-promoted-admin'`)).To(Equal(1),
				"a level-admin user must survive deletion even if an earlier computed candidate list named them")
		})

		It("should block a sync and write nothing when calculated deletions exceed the configured threshold", func() {
			os.Setenv(services.EnvMaxDeletePercent, "1")
			defer os.Unsetenv(services.EnvMaxDeletePercent)

			for i := 0; i < 5; i++ {
				name := "sync-bulk-" + string(rune('a'+i))
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $1, $2, $1, 'level-5', '')
					ON CONFLICT (id) DO UPDATE SET is_active = true, deactivated_at = NULL
				`, name, name+"@test.com")
				Expect(err).NotTo(HaveOccurred())
			}

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
			Expect(w.Body.String()).To(ContainSubstring("review"))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5), "nothing may be deleted once the guard trips")
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(0), "and nothing may be created/updated either -- the whole sync is one transaction")
		})

		It("should reject a non-admin", func() {
			w := doSync(memberToken)
			Expect(w.Code).To(Equal(http.StatusForbidden))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(0))
		})

		It("should reject an unauthenticated request", func() {
			req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/organization-provider/sync", nil)
			Expect(err).NotTo(HaveOccurred())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusUnauthorized))
		})

		It("should return 409 when a sync is already running", func() {
			providerGate = make(chan struct{})

			done := make(chan int, 1)
			go func() {
				defer GinkgoRecover()
				done <- doSync(adminToken).Code
			}()

			// Wait until the first sync is inside the provider call and holding the lock.
			Eventually(providerHit, "5s").Should(Receive())

			second := doSync(adminToken)
			Expect(second.Code).To(Equal(http.StatusConflict), second.Body.String())

			var body dto.ErrorResponse
			Expect(json.Unmarshal(second.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Message).NotTo(BeEmpty(), "the frontend reads message, not error -- an empty message falls back to the generic '409 Conflict' status text")

			close(providerGate)
			Eventually(done, "10s").Should(Receive(Equal(http.StatusOK)))
		})

		It("should write nothing when the provider fails", func() {
			providerStatus = http.StatusInternalServerError
			providerBody = `{"error":"upstream exploded"}`

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusBadGateway))
			Expect(w.Body.String()).NotTo(ContainSubstring("upstream exploded"))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id LIKE 'sync-team-%'`)).To(Equal(0))
		})

		It("should write nothing when the snapshot breaches the contract", func() {
			providerBody = `{
				"contractVersion": "2.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [{"id": "sync-team-1", "name": "Alcatraz"}],
				"users": [{"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"}],
				"memberships": []
			}`

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusBadGateway))
			Expect(w.Body.String()).To(ContainSubstring("contract version"))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id LIKE 'sync-team-%'`)).To(Equal(0))
		})

		It("should refuse to sync when the API token is not configured", func() {
			os.Unsetenv(dataprovider.EnvAPIToken)

			// Rebuild the router with a fetcher constructed against the now-incomplete environment.
			providerRepo := postgres.NewOrganizationProviderRepository(db)
			userRepo := postgres.NewUserRepository(db)
			teamRepo := postgres.NewTeamRepository(db)
			cfg, err := dataprovider.LoadConfig()
			Expect(err).To(HaveOccurred(), "a base URL without a token is an incomplete configuration")
			Expect(cfg).To(BeNil())
			syncService := services.NewOrganizationSyncService(providerRepo, nil, userRepo, teamRepo)
			jwtService := services.NewJWTService()
			router = gin.New()
			v1.SetupOrganizationProviderRoutes(router, syncService, postgres.NewOrganizationRepository(db), jwtService)

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(w.Body.String()).To(ContainSubstring("not configured"))
		})

		It("should never disclose the provider token", func() {
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).NotTo(ContainSubstring(providerAPIToken))
		})
	})

	// The guard is what stops a bad snapshot from emptying the org; the override
	// is the one deliberate way past it. These specs pin down both halves: what
	// a held sync reports (and that it reports it without writing), and exactly
	// how much an override is allowed to waive.
	Describe("Mass-deletion hold and admin override", func() {
		// seedBulkUsers creates non-protected users absent from every snapshot
		// this suite serves, so they are all deletion candidates. With the
		// threshold pinned to 1% below, any one of them trips the guard.
		seedBulkUsers := func(n int) {
			for i := 0; i < n; i++ {
				name := "sync-bulk-" + string(rune('a'+i))
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $1, $2, $1, 'level-5', '')
					ON CONFLICT (id) DO UPDATE SET is_active = true, deactivated_at = NULL
				`, name, name+"@test.com")
				Expect(err).NotTo(HaveOccurred())
			}
		}

		decodeHold := func(w *httptest.ResponseRecorder) dto.MassDeletionHoldResponseDTO {
			var body dto.MassDeletionHoldResponseDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed(), w.Body.String())
			return body
		}

		BeforeEach(func() {
			// A strict threshold makes the guard trip on this suite's small
			// fixture populations. The production default (20%) is untouched.
			os.Setenv(services.EnvMaxDeletePercent, "1")
		})

		AfterEach(func() {
			os.Setenv(services.EnvMaxDeletePercent, "100")
		})

		It("should report the deletion counts, percentages and threshold when it holds a sync", func() {
			seedBulkUsers(5)
			_, err := db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-doomed-team', 'Doomed')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-doomed-team', 'sync-bulk-a')`)
			Expect(err).NotTo(HaveOccurred())

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())

			body := decodeHold(w)
			Expect(body.Code).To(Equal(dto.CodeMassDeletionHold), "the UI recognises a hold by its typed code, not by message text")
			Expect(body.Applied).To(BeFalse())
			Expect(body.MassDeletion).NotTo(BeNil())

			report := body.MassDeletion
			Expect(report.Threshold).To(Equal(1.0))

			// 5 eligible users exist, all 5 are absent from the snapshot.
			Expect(report.Users.Existing).To(Equal(5), "the denominator is the eligible existing population")
			Expect(report.Users.Deleting).To(Equal(5), "the numerator is what the sync proposes to delete")
			Expect(report.Users.Percent).To(Equal(100.0))
			Expect(report.Users.Kind).To(Equal("deleted"))
			Expect(report.Users.ContributesToHold).To(BeTrue())
			Expect(report.Users.ExceedsThreshold).To(BeTrue())
			// The diagnostic half of the report: what the provider actually sent.
			// snapshotWithLevels carries 4 users, of which only 2 are importable --
			// Incoming must count the raw payload, not the filtered/importable
			// subset, or a payload with many bad-level records would misreport
			// as an incomplete one.
			Expect(report.Users.Incoming).To(Equal(4), "incoming counts every record the provider sent, including ones the filter later skips")

			// Only sync-doomed-team exists and it is absent from the snapshot.
			Expect(report.Teams.Existing).To(Equal(1))
			Expect(report.Teams.Deleting).To(Equal(1))
			Expect(report.Teams.Percent).To(Equal(100.0))
			Expect(report.Teams.Incoming).To(Equal(2), "the snapshot names 2 teams, neither of which exists yet")

			// Memberships are a cascade of those deletions, reported but never
			// a reason to hold.
			Expect(report.Memberships).NotTo(BeNil())
			Expect(report.Memberships.Kind).To(Equal("cascaded"))
			Expect(report.Memberships.Existing).To(Equal(1))
			Expect(report.Memberships.Deleting).To(Equal(1))
			Expect(report.Memberships.ContributesToHold).To(BeFalse())
		})

		It("should not count a missing user's membership in a surviving team as cascaded", func() {
			// sync-team-1 is one of snapshotWithLevels' own teams, so it survives
			// this sync -- its membership is reconciled by an explicit per-team
			// DELETE (MembershipsRemoved), not by an FK cascade on a hard-deleted
			// team or user. The cascade metric must not double-label that row.
			seedBulkUsers(5)
			_, err := db.Exec(`INSERT INTO teams (id, name) VALUES ('sync-team-1', 'Alcatraz')`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sync-team-1', 'sync-bulk-a')`)
			Expect(err).NotTo(HaveOccurred())

			held := doSync(adminToken)
			Expect(held.Code).To(Equal(http.StatusConflict), held.Body.String())

			report := decodeHold(held).MassDeletion
			Expect(report.Memberships).NotTo(BeNil())
			Expect(report.Memberships.Existing).To(Equal(1))
			Expect(report.Memberships.Deleting).To(Equal(0),
				"sync-team-1 survives, so sync-bulk-a's row there is pruned by the explicit per-team reconciliation, not an FK cascade")

			// Confirm the row is still genuinely removed -- just correctly
			// attributed to MembershipsRemoved instead of the cascade metric.
			w := doOverrideSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.MembershipsRemoved).To(Equal(1))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-bulk-a' AND is_active`)).To(BeZero())
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-bulk-a' AND is_active = false`)).To(Equal(1),
				"the membership is kept as inactive history")
		})

		It("should write nothing at all while held", func() {
			seedBulkUsers(5)

			Expect(doSync(adminToken).Code).To(Equal(http.StatusConflict))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5), "nothing may be deleted once the guard trips")
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(0), "and nothing may be created either -- the whole sync is one transaction")
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id LIKE 'sync-team-%'`)).To(Equal(0))
		})

		It("should never disclose the provider token in a hold response", func() {
			seedBulkUsers(5)
			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusConflict))
			Expect(w.Body.String()).NotTo(ContainSubstring(providerAPIToken))
		})

		It("should apply the sync when an admin explicitly overrides the hold", func() {
			seedBulkUsers(5)

			Expect(doSync(adminToken).Code).To(Equal(http.StatusConflict), "the plain sync must still hold")

			w := doOverrideSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.MassDeletionOverridden).To(BeTrue())
			Expect(result.MassDeletion).NotTo(BeNil(), "an overridden run reports what it waived")
			Expect(result.MassDeletion.Users.Deleting).To(Equal(5))
			Expect(result.UsersDeactivated).To(Equal(5))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%' AND is_active`)).To(BeZero())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%' AND is_active = false`)).To(Equal(5), "deactivated, never deleted")
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(2), "the snapshot's users are imported by the overridden run")
		})

		It("should refuse a stale override and hold again on the fresh counts when the deletion scope changes between the hold and the confirmation", func() {
			seedBulkUsers(5)

			held := doSync(adminToken)
			Expect(held.Code).To(Equal(http.StatusConflict))
			var staleHold dto.MassDeletionHoldResponseDTO
			Expect(json.Unmarshal(held.Body.Bytes(), &staleHold)).To(Succeed())
			Expect(staleHold.MassDeletion.Users.Existing).To(Equal(5))
			Expect(staleHold.MassDeletion.Users.Deleting).To(Equal(5))

			// The deletion scope changes after the admin reviewed this report but
			// before they confirmed the override -- the exact race the fix closes.
			// A real cause would be the provider's snapshot changing; growing the
			// current non-protected population produces the identical effect on
			// the guard's fresh recomputation without needing a second payload.
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash) VALUES
				('sync-more-a', 'sync-more-a', 'sync-more-a@test.com', 'sync-more-a', 'level-5', ''),
				('sync-more-b', 'sync-more-b', 'sync-more-b@test.com', 'sync-more-b', 'level-5', ''),
				('sync-more-c', 'sync-more-c', 'sync-more-c@test.com', 'sync-more-c', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())

			staleConfirmation, err := json.Marshal(map[string]any{
				"overrideMassDeletion": true,
				"confirmedMassDeletion": map[string]int{
					"usersExisting": staleHold.MassDeletion.Users.Existing,
					"usersDeleting": staleHold.MassDeletion.Users.Deleting,
					"teamsExisting": staleHold.MassDeletion.Teams.Existing,
					"teamsDeleting": staleHold.MassDeletion.Teams.Deleting,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			w := doSyncWithBody(adminToken, string(staleConfirmation))
			Expect(w.Code).To(Equal(http.StatusConflict), "a stale confirmation must be refused, not silently applied to whatever the fresh count now shows")

			var freshHold dto.MassDeletionHoldResponseDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &freshHold)).To(Succeed())
			Expect(freshHold.MassDeletion.Users.Existing).To(Equal(8), "the fresh report reflects the changed population, for the admin to re-review")
			Expect(freshHold.MassDeletion.Users.Deleting).To(Equal(8))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%' OR id LIKE 'sync-more-%'`)).To(Equal(8), "nothing may be deleted on a refused override")

			// Confirming the fresh counts instead succeeds.
			freshConfirmation, err := json.Marshal(map[string]any{
				"overrideMassDeletion": true,
				"confirmedMassDeletion": map[string]int{
					"usersExisting": freshHold.MassDeletion.Users.Existing,
					"usersDeleting": freshHold.MassDeletion.Users.Deleting,
					"teamsExisting": freshHold.MassDeletion.Teams.Existing,
					"teamsDeleting": freshHold.MassDeletion.Teams.Deleting,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			w2 := doSyncWithBody(adminToken, string(freshConfirmation))
			Expect(w2.Code).To(Equal(http.StatusOK), w2.Body.String())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE (id LIKE 'sync-bulk-%' OR id LIKE 'sync-more-%') AND is_active`)).To(BeZero())
		})

		It("should scope the override to the one request carrying it", func() {
			seedBulkUsers(5)
			Expect(doOverrideSync(adminToken).Code).To(Equal(http.StatusOK))

			// Re-seed the same over-threshold condition and sync normally: the
			// previous override must not have relaxed anything for this run.
			seedBulkUsers(5)
			Expect(doSync(adminToken).Code).To(Equal(http.StatusConflict))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should not override when the flag is false", func() {
			seedBulkUsers(5)

			w := doSyncWithBody(adminToken, `{"overrideMassDeletion": false}`)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should reject a non-admin asking for an override, and write nothing", func() {
			seedBulkUsers(5)

			w := doOverrideSync(memberToken)
			Expect(w.Code).To(Equal(http.StatusForbidden), w.Body.String())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(BeZero())
		})

		It("should reject an unauthenticated override", func() {
			seedBulkUsers(5)

			req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/organization-provider/sync", strings.NewReader(`{"overrideMassDeletion": true}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should still protect admin, demo and E2E fixtures when overriding", func() {
			seedBulkUsers(5)
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash) VALUES
				('demo', 'demo', 'demo@teams360.demo', 'Demo User', 'level-5', ''),
				('vp', 'vp', 'vp@teams360.demo', 'VP', 'level-1', ''),
				('e2e_demo', 'e2e_demo', 'e2e_demo@teams360.demo', 'E2E Demo', 'level-5', '')
			`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`INSERT INTO teams (id, name) VALUES ('team-phoenix', 'Phoenix Squad'), ('e2e_team1', 'E2E Team')`)
			Expect(err).NotTo(HaveOccurred())

			w := doOverrideSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			for _, id := range []string{"admin", "demo", "vp", "e2e_demo"} {
				Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = $1`, id)).To(Equal(1), id+" must survive an overridden sync")
			}
			for _, id := range []string{"team-phoenix", "e2e_team1"} {
				Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = $1`, id)).To(Equal(1), id+" must survive an overridden sync")
			}
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%' AND is_active`)).To(BeZero(), "the override still deactivates the non-protected records")
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5), "...but never deletes them")
		})

		It("should still preserve health-check history when overriding", func() {
			seedBulkUsers(1)
			_, err := db.Exec(`
				INSERT INTO health_check_sessions (id, user_id, team_id, date, assessment_period, completed)
				VALUES ('sync-override-session', 'sync-bulk-a', 'sync-doomed-team', '2026-09-01', '2026 - 1st Half', true)
			`)
			Expect(err).NotTo(HaveOccurred())

			Expect(doOverrideSync(adminToken).Code).To(Equal(http.StatusOK))

			Expect(countRows(`SELECT COUNT(*) FROM health_check_sessions WHERE id = 'sync-override-session'`)).To(Equal(1),
				"history must survive an overridden sync exactly as it survives a normal one")
		})

		It("should still validate the snapshot when an override is requested", func() {
			seedBulkUsers(5)
			providerBody = `{
				"contractVersion": "2.0",
				"generatedAt": "2026-08-31T05:04:48.240Z",
				"teams": [{"id": "sync-team-1", "name": "Alcatraz"}],
				"users": [{"id": "sync-user-1", "username": "syncalice", "displayName": "Alice", "email": "syncalice@test.com", "hierarchyLevelId": "level-3"}],
				"memberships": []
			}`

			w := doOverrideSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusBadGateway), "an override waives the deletion guard, never validation")
			Expect(w.Body.String()).To(ContainSubstring("contract version"))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should still fail an override when the provider itself fails", func() {
			seedBulkUsers(5)
			providerStatus = http.StatusInternalServerError
			providerBody = `{"error":"upstream exploded"}`

			w := doOverrideSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusBadGateway))
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should reject a malformed body rather than silently syncing", func() {
			seedBulkUsers(5)

			w := doSyncWithBody(adminToken, `{"overrideMassDeletion": "yes"}`)
			Expect(w.Code).To(Equal(http.StatusBadRequest), w.Body.String())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-bulk-%'`)).To(Equal(5))
		})

		It("should leave a safe sync untouched by any of this", func() {
			os.Setenv(services.EnvMaxDeletePercent, "100")

			w := doSync(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			var result services.SyncResult
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result.MassDeletionOverridden).To(BeFalse(), "a sync under the threshold is not an override")
			Expect(result.MassDeletion).To(BeNil())
			Expect(result.UsersSynced).To(Equal(2))
		})
	})

	Describe("Organization provider settings", func() {
		It("should report configuration status without ever carrying a token field", func() {
			w := getSettings(adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).NotTo(ContainSubstring(providerAPIToken))

			var settings map[string]any
			Expect(json.Unmarshal(w.Body.Bytes(), &settings)).To(Succeed())
			Expect(settings["baseUrlConfigured"]).To(BeTrue())
			Expect(settings["tokenConfigured"]).To(BeTrue())
			Expect(settings["readyToSync"]).To(BeTrue())

			for _, forbidden := range []string{"apiToken", "token", "apiTokenEncrypted", "configured", "tokenUpdatedAt"} {
				Expect(settings).NotTo(HaveKey(forbidden))
			}
		})

		It("should reject a non-admin", func() {
			Expect(getSettings(memberToken).Code).To(Equal(http.StatusForbidden))
		})

		It("should reject an unauthenticated request", func() {
			req, err := http.NewRequest(http.MethodGet, "/api/v1/admin/settings/organization-provider", nil)
			Expect(err).NotTo(HaveOccurred())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusUnauthorized))
		})
	})
})
