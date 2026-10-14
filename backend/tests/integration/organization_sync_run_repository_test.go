package integration_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Exercises the Phase 1 safety foundation against a real database: durable
// result persistence, the migration's CHECK constraints, the advisory lock's
// visibility in pg_locks, and hold_active's behaviour across a real
// success/blocked cycle. Complements organization_sync_lock_and_recording_test.go
// (service-unit, fakes) by proving the same properties actually hold at the
// SQL layer -- CHECK constraints and pg_locks in particular cannot be proven
// any other way.
var _ = Describe("Integration: Organization Sync Run Repository", func() {
	var (
		db             *sql.DB
		cleanup        func()
		providerRepo   *postgres.OrganizationProviderRepository
		providerServer *httptest.Server
	)

	// Reuses the one-user, one-team snapshot shape established by the
	// sibling threshold test: deleting everyone else is what makes both a
	// clean success and a mass-deletion hold observable.
	const oneUserSnapshot = `{
	  "contractVersion": "1.0",
	  "generatedAt": "2026-08-31T05:04:48.240Z",
	  "teams": [{"id": "run-repo-team", "name": "Run Repo Team"}],
	  "users": [
	    {"id": "run-repo-snapshot-user", "username": "runreposnapshotuser", "displayName": "Run Repo Snapshot User", "email": "run-repo-snapshot-user@test.com", "hierarchyLevelId": "level-5"}
	  ],
	  "memberships": []
	}`

	seedUsers := func(n int) {
		for i := 0; i < n; i++ {
			name := "run-repo-user-" + string(rune('a'+i))
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1, $1, $2, $1, 'level-5', '')
			`, name, name+"@test.com")
			Expect(err).NotTo(HaveOccurred())
		}
	}

	countRows := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	BeforeEach(func() {
		os.Unsetenv(services.EnvMaxDeletePercent)
		db, cleanup = testhelpers.SetupTestDatabase()
		providerRepo = postgres.NewOrganizationProviderRepository(db)

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
	})

	AfterEach(func() {
		providerServer.Close()
		cleanup()
		os.Unsetenv(services.EnvMaxDeletePercent)
		os.Unsetenv(dataprovider.EnvBaseURL)
		os.Unsetenv(dataprovider.EnvAPIToken)
	})

	newService := func(maxDeletePercent string) *services.OrganizationSyncService {
		os.Setenv(services.EnvMaxDeletePercent, maxDeletePercent)
		cfg, err := dataprovider.LoadConfig()
		Expect(err).NotTo(HaveOccurred())
		client, err := dataprovider.NewClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		return services.NewOrganizationSyncService(providerRepo, client, nil, nil,
			services.WithSyncLocker(postgres.NewOrgSyncLocker(db)),
			services.WithSyncRunRecorder(providerRepo),
			services.WithInstanceID("integration-test-instance"),
		)
	}

	It("starts empty: org_sync_runs has no row until a sync request is recorded", func() {
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(0))
	})

	It("rejects an invalid writes_status via the CHECK constraint rather than silently accepting it", func() {
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at, writes_status)
			VALUES (gen_random_uuid(), 'manual', 'failed', NOW(), NOW(), 'maybe')
		`)
		Expect(err).To(HaveOccurred())
	})

	It("rejects an invalid skip_reason via the CHECK constraint", func() {
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at, skip_reason)
			VALUES (gen_random_uuid(), 'scheduled', 'skipped', NOW(), NOW(), 'because_i_felt_like_it')
		`)
		Expect(err).To(HaveOccurred())
	})

	It("records a successful sync as applied, folded atomically with the destructive writes", func() {
		service := newService("90")

		result, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerScheduled})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		last, err := providerRepo.GetOrgSyncLastRun(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(last.LastAttempt).NotTo(BeNil())
		Expect(last.LastAttempt.Status).To(Equal(orgprovider.StatusSuccess))
		Expect(last.LastAttempt.WritesStatus).To(Equal(orgprovider.WritesApplied))
		Expect(last.LastAttempt.Trigger).To(Equal(orgprovider.TriggerScheduled))
		Expect(last.HoldActive).To(BeFalse())
	})

	It("records a blocked sync durably even though its destructive-write transaction rolled back", func() {
		seedUsers(20) // makes the one-user snapshot propose deleting ~95%
		service := newService("20")

		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{Trigger: orgprovider.TriggerManual})
		Expect(err).To(HaveOccurred())

		// No destructive writes actually landed -- the guard's own point.
		Expect(countRows(`SELECT COUNT(*) FROM users WHERE id LIKE 'run-repo-user-%'`)).To(Equal(20))

		// The blocked result survived the very rollback that just proved it
		// wrote nothing -- proof that RecordOrgSyncAttempt's write went
		// through a SEPARATE transaction, not the doomed one.
		last, err2 := providerRepo.GetOrgSyncLastRun(context.Background())
		Expect(err2).NotTo(HaveOccurred())
		Expect(last.LastAttempt).NotTo(BeNil())
		Expect(last.LastAttempt.Status).To(Equal(orgprovider.StatusBlocked))
		Expect(last.LastAttempt.WritesStatus).To(Equal(orgprovider.WritesNone))
		Expect(last.HoldActive).To(BeTrue())
		Expect(last.LastAttempt.UsersDeleting).NotTo(BeNil())
	})

	It("clears the durable hold when a later sync succeeds", func() {
		seedUsers(20)
		service := newService("20")

		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{})
		Expect(err).To(HaveOccurred())
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE hold_active = true`)).To(Equal(1))

		// While held, thresholdForRun reuses the FROZEN 20% -- raising the
		// env var here would have no effect, which is the guard's entire
		// point (an admin cannot answer a hold by loosening the limit). The
		// realistic way data actually stops exceeding a frozen threshold is
		// the underlying population changing upstream: simulate that by
		// removing the runaway users directly, so the next attempt proposes
		// deleting nothing and cannot trip the guard.
		_, err = db.Exec(`DELETE FROM users WHERE id LIKE 'run-repo-user-%'`)
		Expect(err).NotTo(HaveOccurred())

		_, err = service.SyncWithOptions(context.Background(), services.SyncOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE hold_active = true`)).To(Equal(0))
	})

	It("leaves hold_active untouched when a LATER attempt fails for an unrelated technical reason", func() {
		seedUsers(20)
		service := newService("20")

		_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{})
		Expect(err).To(HaveOccurred())
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE hold_active = true`)).To(Equal(1))

		// Break the provider so the NEXT attempt fails before ever reaching
		// the guard again.
		providerServer.Close()
		_, err = service.SyncWithOptions(context.Background(), services.SyncOptions{})
		Expect(err).To(HaveOccurred())

		// The hold from the FIRST attempt must still be active -- an
		// unrelated technical failure must never silently clear it.
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE hold_active = true`)).To(Equal(1))
	})

	It("shows the advisory lock in pg_locks while a sync is gated, and releases it after", func() {
		// The provider handler blocks on a channel until the test signals it,
		// holding the sync (and therefore the lock) open long enough to
		// observe pg_locks directly -- the cheapest way to catch the
		// connection-affinity bug, which otherwise fails with no error
		// anywhere. This is the manual verification the plan calls for,
		// automated where the environment allows it.
		release := make(chan struct{})
		reached := make(chan struct{}, 1)
		gatedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case reached <- struct{}{}:
			default:
			}
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(oneUserSnapshot))
		}))
		defer gatedServer.Close()
		os.Setenv(dataprovider.EnvBaseURL, gatedServer.URL)

		service := newService("90")
		done := make(chan error, 1)
		go func() {
			_, err := service.SyncWithOptions(context.Background(), services.SyncOptions{})
			done <- err
		}()

		// Postgres's documented pg_locks encoding for a two-key advisory lock
		// (pg_try_advisory_lock(key1, key2)): classid = key1, objid = key2,
		// objsubid = 2 (objsubid = 1 is reserved for the single-bigint form).
		// advisoryLockClassID/advisoryLockOrgSync in organization_sync_locker.go
		// are 360 and 1 -- kept in sync with the literals here deliberately,
		// since a real assertion, not a tautology, is the point of this test.
		Eventually(reached, "2s").Should(Receive())
		Eventually(func() int {
			return countRows(`SELECT COUNT(*) FROM pg_locks WHERE locktype = 'advisory' AND classid = 360 AND objid = 1 AND objsubid = 2`)
		}, "2s").Should(Equal(1), "expected the (360, 1) advisory lock to be visible in pg_locks while the sync is gated")

		close(release)
		Expect(<-done).NotTo(HaveOccurred())

		Eventually(func() int {
			return countRows(`SELECT COUNT(*) FROM pg_locks WHERE locktype = 'advisory' AND classid = 360 AND objid = 1 AND objsubid = 2`)
		}, "2s").Should(Equal(0), "expected the advisory lock to be released after the sync completed")
	})
})
