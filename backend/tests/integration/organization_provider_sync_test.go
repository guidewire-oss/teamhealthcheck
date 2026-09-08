package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
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
		v1.SetupOrganizationProviderRoutes(router, syncService, jwtService)
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
			Expect(result.UsersDeleted).To(BeZero(), "nothing outside the snapshot is missing on a repeat run")
			Expect(result.TeamsDeleted).To(BeZero())

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'sync-user-%'`)).To(Equal(2))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id LIKE 'sync-team-%'`)).To(Equal(first))
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
				`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-team-1' AND user_id = 'sync-stale'`,
			)).To(Equal(0), "a member the provider dropped should be removed")

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
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id IN ('sync-team-1','sync-team-2')`)).To(Equal(0), "an explicit empty membership list must clear existing rows, not be ignored")
		})

		It("should hard-delete a non-protected user missing from the complete snapshot", func() {
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
			Expect(result.UsersDeleted).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-departed'`)).To(Equal(0))
		})

		It("should hard-delete a non-protected team missing from the complete snapshot, cascading its memberships", func() {
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
			Expect(result.TeamsDeleted).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-gone-team'`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM team_members WHERE team_id = 'sync-gone-team'`)).To(Equal(0), "cascade must remove the deleted team's memberships")
		})

		It("should hard-delete a user or team that owns an action item, cascading it, and report the cascade count", func() {
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
			Expect(result.UsersDeleted).To(BeNumerically(">=", 1))
			Expect(result.TeamsDeleted).To(Equal(1))
			Expect(result.ActionItemsDeleted).To(Equal(1), "the cascade must be counted and surfaced, since it can't be prevented")

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-owner'`)).To(Equal(0), "the provider is authoritative -- owning an action item does not exempt a record from deletion")
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-owned-team'`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM action_items WHERE id = 'sync-action-1'`)).To(Equal(0), "cascaded away by the existing FK -- not something this sync can prevent without a schema change")
		})

		It("should never delete or alter health-check history when a user/team is hard-deleted", func() {
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
			Expect(result.UsersDeleted).To(Equal(1))
			Expect(result.TeamsDeleted).To(Equal(1))

			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id = 'sync-history-user'`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'sync-history-team'`)).To(Equal(0))

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
			Expect(result.UsersDeleted).To(BeZero(), "protected fixtures are excluded from the deletion scope entirely")

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

		It("should block a sync and write nothing when calculated deletions exceed the configured threshold", func() {
			os.Setenv(services.EnvMaxDeletePercent, "1")
			defer os.Unsetenv(services.EnvMaxDeletePercent)

			for i := 0; i < 5; i++ {
				name := "sync-bulk-" + string(rune('a'+i))
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $1, $2, $1, 'level-5', '')
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
			v1.SetupOrganizationProviderRoutes(router, syncService, jwtService)

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
