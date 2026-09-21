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

// Exercises the admin-configurable mass-deletion threshold end to end: the
// settings endpoints, the app_settings column behind them, and the precedence
// the sync applies when both a saved value and the environment variable exist.
var _ = Describe("Integration: Organization Sync Deletion Threshold", func() {
	const thresholdPath = "/api/v1/admin/settings/organization-provider/deletion-threshold"

	var (
		db             *sql.DB
		router         *gin.Engine
		cleanup        func()
		adminToken     string
		memberToken    string
		providerServer *httptest.Server
	)

	// A snapshot reporting one team and exactly one user: against a populated
	// database this proposes deleting nearly everyone else, which is what
	// makes the threshold observable. It cannot report zero users -- the
	// snapshot contract (pkg/orgsnapshot/validate.go) rejects an empty users
	// array outright as a malformed payload, before the sync ever reaches the
	// mass-deletion guard this file is testing. The one user it does report
	// is never seeded into the database beforehand, so it contributes nothing
	// to the deletion count; it only gets created by the sync, exactly like
	// any other ordinary incoming record.
	const oneUserSnapshot = `{
	  "contractVersion": "1.0",
	  "generatedAt": "2026-08-31T05:04:48.240Z",
	  "teams": [{"id": "threshold-team", "name": "Threshold Team"}],
	  "users": [
	    {"id": "threshold-snapshot-user", "username": "thresholdsnapshotuser", "displayName": "Threshold Snapshot User", "email": "threshold-snapshot-user@test.com", "hierarchyLevelId": "level-5"}
	  ],
	  "memberships": []
	}`

	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		var req *http.Request
		var err error
		if body == "" {
			req, err = http.NewRequest(method, path, nil)
		} else {
			req, err = http.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		Expect(err).NotTo(HaveOccurred())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	countRows := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	// seedUsers inserts n deletable users, so a snapshot that omits them
	// proposes a 100% deletion.
	seedUsers := func(n int) {
		for i := 0; i < n; i++ {
			name := "threshold-user-" + string(rune('a'+i))
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1, $1, $2, $1, 'level-5', '')
			`, name, name+"@test.com")
			Expect(err).NotTo(HaveOccurred())
		}
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-integration-tests")
		os.Unsetenv(services.EnvMaxDeletePercent)
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		providerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != providerAPIToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(oneUserSnapshot))
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

		orgRepo := postgres.NewOrganizationRepository(db)
		providerRepo := postgres.NewOrganizationProviderRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)

		cfg, err := dataprovider.LoadConfig()
		Expect(err).NotTo(HaveOccurred())
		client, err := dataprovider.NewClient(cfg)
		Expect(err).NotTo(HaveOccurred())

		syncService := services.NewOrganizationSyncService(providerRepo, client, userRepo, teamRepo,
			services.WithDeleteThresholdStore(orgRepo))

		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, syncService, orgRepo, jwtService)
	})

	AfterEach(func() {
		providerServer.Close()
		cleanup()
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv(services.EnvMaxDeletePercent)
		os.Unsetenv(dataprovider.EnvBaseURL)
		os.Unsetenv(dataprovider.EnvAPIToken)
	})

	decode := func(w *httptest.ResponseRecorder) dto.OrgSyncDeletionThreshold {
		var body dto.OrgSyncDeletionThreshold
		Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
		return body
	}

	It("reports the 20% default on an installation that has configured nothing", func() {
		body := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
		Expect(body.MaxDeletePercent).To(Equal(20.0))
		Expect(body.Source).To(Equal(services.ThresholdSourceDefault))
	})

	It("round-trips a saved value through app_settings and prefers it over the environment", func() {
		os.Setenv(services.EnvMaxDeletePercent, "90")

		Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 33}`).Code).
			To(Equal(http.StatusOK))

		Expect(countRows(`SELECT COUNT(*) FROM app_settings WHERE org_sync_max_delete_percent = 33`)).To(Equal(1))

		body := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
		Expect(body.MaxDeletePercent).To(Equal(33.0))
		Expect(body.Source).To(Equal(services.ThresholdSourceAdmin))
	})

	It("rejects an out-of-range value and leaves the stored threshold alone", func() {
		Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 30}`).Code).
			To(Equal(http.StatusOK))

		for _, body := range []string{`{"maxDeletePercent": 0}`, `{"maxDeletePercent": 101}`, `{}`} {
			Expect(request(http.MethodPut, thresholdPath, adminToken, body).Code).
				To(Equal(http.StatusBadRequest), body)
		}

		Expect(decode(request(http.MethodGet, thresholdPath, adminToken, "")).MaxDeletePercent).To(Equal(30.0))
	})

	It("refuses a non-admin on both read and write", func() {
		Expect(request(http.MethodGet, thresholdPath, memberToken, "").Code).To(Equal(http.StatusForbidden))
		Expect(request(http.MethodPut, thresholdPath, memberToken, `{"maxDeletePercent": 99}`).Code).
			To(Equal(http.StatusForbidden))
		Expect(countRows(`SELECT COUNT(*) FROM app_settings WHERE org_sync_max_delete_percent IS NOT NULL`)).To(Equal(0))
	})

	It("holds a sync that exceeds the saved threshold and writes nothing", func() {
		// 90% in the environment would let this sync through; the saved 10%
		// must be what decides.
		os.Setenv(services.EnvMaxDeletePercent, "90")
		seedUsers(5)
		Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 10}`).Code).
			To(Equal(http.StatusOK))

		w := request(http.MethodPost, "/api/v1/admin/organization-provider/sync", adminToken, "")
		Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())

		var hold dto.MassDeletionHoldResponseDTO
		Expect(json.Unmarshal(w.Body.Bytes(), &hold)).To(Succeed())
		Expect(hold.Applied).To(BeFalse())
		Expect(hold.MassDeletion).NotTo(BeNil())
		Expect(hold.MassDeletion.Threshold).To(Equal(10.0))

		Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'threshold-user-%'`)).To(Equal(5))
		Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'threshold-team'`)).To(Equal(0))
	})

	Describe("locking the threshold for the lifetime of a sync attempt", func() {
		const syncPath = "/api/v1/admin/organization-provider/sync"
		const holdPath = "/api/v1/admin/organization-provider/sync/hold"

		// hold runs a sync that trips the guard at 10% and returns the counts
		// the admin would be shown.
		hold := func() dto.MassDeletionHoldResponseDTO {
			seedUsers(5)
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 10}`).Code).
				To(Equal(http.StatusOK))

			w := request(http.MethodPost, syncPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
			var body dto.MassDeletionHoldResponseDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			return body
		}

		It("refuses to raise the threshold while a hold is unresolved", func() {
			hold()

			w := request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())

			var body dto.ErrorResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Code).To(Equal(dto.CodeThresholdLocked))

			// The stored value is untouched, and a reload still sees the lock.
			read := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
			Expect(read.MaxDeletePercent).To(Equal(10.0))
			Expect(read.Locked).To(BeTrue())
			Expect(read.LockReason).To(Equal(dto.ThresholdLockHeld))
		})

		It("keeps holding a retry after an attempted threshold change", func() {
			hold()
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`).Code).
				To(Equal(http.StatusConflict))

			// The retry is judged against the frozen 10%, so it is held again
			// and still writes nothing.
			w := request(http.MethodPost, syncPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'threshold-user-%'`)).To(Equal(5))
		})

		It("lets Sync Anyway through and unlocks once the hold is resolved", func() {
			held := hold()
			Expect(held.MassDeletion).NotTo(BeNil())

			override, err := json.Marshal(map[string]any{
				"overrideMassDeletion": true,
				"confirmedMassDeletion": map[string]int{
					"usersExisting": held.MassDeletion.Users.Existing,
					"usersDeleting": held.MassDeletion.Users.Deleting,
					"teamsExisting": held.MassDeletion.Teams.Existing,
					"teamsDeleting": held.MassDeletion.Teams.Deleting,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			w := request(http.MethodPost, syncPath, adminToken, string(override))
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'threshold-user-%'`)).To(Equal(0))

			Expect(decode(request(http.MethodGet, thresholdPath, adminToken, "")).Locked).To(BeFalse())
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 50}`).Code).
				To(Equal(http.StatusOK))
		})

		It("unlocks when the admin dismisses the hold without applying it", func() {
			hold()

			Expect(decode(request(http.MethodDelete, holdPath, adminToken, "")).Locked).To(BeFalse())
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 50}`).Code).
				To(Equal(http.StatusOK))

			// Dismissing reviews the hold away; it never applies the sync.
			Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'threshold-user-%'`)).To(Equal(5))
		})
	})

	It("lets the same sync through once the saved threshold allows it", func() {
		seedUsers(5)
		Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`).Code).
			To(Equal(http.StatusOK))

		w := request(http.MethodPost, "/api/v1/admin/organization-provider/sync", adminToken, "")
		Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

		Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'threshold-user-%'`)).To(Equal(0))
		Expect(countRows(`SELECT COUNT(*) FROM teams WHERE id = 'threshold-team'`)).To(Equal(1))
	})
})
