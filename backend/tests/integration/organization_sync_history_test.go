package integration_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Exercises org_sync_runs as a one-row-per-request log against a real
// database: run IDs, newest-first reads, the org_sync_runs_latest view, the
// durable hold carried on blocked rows, the migration's CHECK constraints and
// the pruning cap. The SQL here cannot be proven any other way.
var _ = Describe("Integration: Organization Sync Runs log", func() {
	var (
		db             *sql.DB
		cleanup        func()
		repo           *postgres.OrganizationProviderRepository
		providerServer *httptest.Server
	)

	const oneUserSnapshot = `{
	  "contractVersion": "1.0",
	  "generatedAt": "2026-08-31T05:04:48.240Z",
	  "teams": [{"id": "history-team", "name": "History Team"}],
	  "users": [
	    {"id": "history-snapshot-user", "username": "historysnapshotuser", "displayName": "History Snapshot User", "email": "history-snapshot-user@test.com", "hierarchyLevelId": "level-5"}
	  ],
	  "memberships": []
	}`

	countRows := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	failedAttempt := func(runID string) orgprovider.OrgSyncAttempt {
		return orgprovider.OrgSyncAttempt{
			RunID: runID, Trigger: orgprovider.TriggerManual, Status: orgprovider.StatusFailed,
			StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
			WritesStatus: orgprovider.WritesNone, Message: "boom",
		}
	}

	blockedAttempt := func(runID string, threshold float64) orgprovider.OrgSyncAttempt {
		return orgprovider.OrgSyncAttempt{
			RunID: runID, Trigger: orgprovider.TriggerManual, Status: orgprovider.StatusBlocked,
			StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
			ThresholdPercent: &threshold,
			WritesStatus:     orgprovider.WritesNone, Message: "blocked",
		}
	}

	BeforeEach(func() {
		os.Unsetenv(services.EnvMaxDeletePercent)
		db, cleanup = testhelpers.SetupTestDatabase()
		repo = postgres.NewOrganizationProviderRepository(db)

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
		return services.NewOrganizationSyncService(repo, client, nil, nil,
			services.WithSyncRunRecorder(repo),
			services.WithInstanceID("history-test-instance"),
		)
	}

	It("records a successful sync as its own row, under the run ID it was given", func() {
		runID := uuid.NewString()
		_, err := newService("90").SyncWithOptions(context.Background(), services.SyncOptions{
			Trigger: orgprovider.TriggerScheduled, RunID: runID,
		})
		Expect(err).NotTo(HaveOccurred())

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].RunID).To(Equal(runID))
		Expect(entries[0].Status).To(Equal(orgprovider.StatusSuccess))
		Expect(entries[0].Trigger).To(Equal(orgprovider.TriggerScheduled))
		Expect(entries[0].WritesStatus).To(Equal(orgprovider.WritesApplied))
		Expect(entries[0].InstanceID).To(Equal("history-test-instance"))
		Expect(entries[0].SkipReason).To(BeEmpty())

		last, err := repo.GetOrgSyncLastRun(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(last.LastAttempt.RunID).To(Equal(runID))
	})

	It("records a blocked sync with the counts that blocked it, and raises the hold on that row", func() {
		for i := 0; i < 20; i++ {
			name := "history-user-" + string(rune('a'+i))
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1, $1, $2, $1, 'level-5', '')
			`, name, name+"@test.com")
			Expect(err).NotTo(HaveOccurred())
		}
		runID := uuid.NewString()

		_, err := newService("20").SyncWithOptions(context.Background(), services.SyncOptions{RunID: runID})
		Expect(err).To(HaveOccurred())

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].RunID).To(Equal(runID))
		Expect(entries[0].Status).To(Equal(orgprovider.StatusBlocked))
		Expect(entries[0].Trigger).To(Equal(orgprovider.TriggerManual))
		Expect(entries[0].WritesStatus).To(Equal(orgprovider.WritesNone))
		Expect(entries[0].UsersDeleting).NotTo(BeNil())
		Expect(*entries[0].UsersDeleting).To(BeNumerically(">", 0))
		Expect(entries[0].Message).NotTo(BeEmpty())
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE run_id = $1 AND hold_active`, runID)).To(Equal(1))
	})

	It("records a failed attempt with its message, and no counts", func() {
		runID := uuid.NewString()
		attempt := failedAttempt(runID)
		attempt.InstanceID = "history-test-instance"
		attempt.WritesStatus = orgprovider.WritesUnknown
		attempt.Message = "failed to commit sync transaction"
		Expect(repo.RecordOrgSyncAttempt(context.Background(), attempt)).To(Succeed())

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Status).To(Equal(orgprovider.StatusFailed))
		Expect(entries[0].WritesStatus).To(Equal(orgprovider.WritesUnknown))
		Expect(entries[0].Message).To(Equal("failed to commit sync transaction"))
		Expect(entries[0].UsersDeleting).To(BeNil(), "a failed attempt never computed a deletion count -- NULL, not 0")

		last, err := repo.GetOrgSyncLastRun(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(last.LastAttempt.RunID).To(Equal(runID))
		Expect(last.LastAttempt.Message).To(Equal("failed to commit sync transaction"))
	})

	It("records a skipped occurrence as its own row without displacing the last attempt", func() {
		attemptID := uuid.NewString()
		Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(attemptID))).To(Succeed())

		skipID := uuid.NewString()
		Expect(repo.RecordOrgSyncSkip(context.Background(), orgprovider.OrgSyncSkip{
			RunID: skipID, Reason: orgprovider.SkipManualRunning, At: time.Now().UTC(),
		})).To(Succeed())

		last, err := repo.GetOrgSyncLastRun(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(last.LastAttempt.RunID).To(Equal(attemptID), "a skip is not an attempt")
		Expect(last.LastSkip.RunID).To(Equal(skipID))
		Expect(last.LastSkip.Reason).To(Equal(orgprovider.SkipManualRunning))

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
		Expect(entries[0].RunID).To(Equal(skipID), "newest first")
		Expect(entries[0].Status).To(Equal(orgprovider.StatusSkipped))
		Expect(entries[0].SkipReason).To(Equal(orgprovider.SkipManualRunning))
		Expect(entries[0].Trigger).To(Equal(orgprovider.TriggerScheduled))
	})

	It("generates a run ID when a direct caller supplies none", func() {
		Expect(repo.RecordOrgSyncSkip(context.Background(), orgprovider.OrgSyncSkip{
			Reason: orgprovider.SkipHoldUnresolved, At: time.Now().UTC(),
		})).To(Succeed())

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		_, parseErr := uuid.Parse(entries[0].RunID)
		Expect(parseErr).NotTo(HaveOccurred())
	})

	It("keeps every request as its own row instead of overwriting the previous one", func() {
		for i := 0; i < 3; i++ {
			Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(uuid.NewString()))).To(Succeed())
		}
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(3))
	})

	It("returns the newest request first, from the repository and from the org_sync_runs_latest view", func() {
		ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
		for _, id := range ids {
			Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(id))).To(Succeed())
		}

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries[0].RunID).To(Equal(ids[2]))
		Expect(entries[2].RunID).To(Equal(ids[0]))

		var top string
		Expect(db.QueryRow(`SELECT run_id::text FROM org_sync_runs_latest LIMIT 1`).Scan(&top)).To(Succeed())
		Expect(top).To(Equal(ids[2]), "the view must put the latest sync first")

		limited, err := repo.ListOrgSyncHistory(context.Background(), 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(limited).To(HaveLen(2))
		Expect(limited[0].RunID).To(Equal(ids[2]))
	})

	Describe("the durable hold", func() {
		It("reports the held threshold from the blocked row, even after a later failure", func() {
			Expect(repo.RecordOrgSyncAttempt(context.Background(), blockedAttempt(uuid.NewString(), 20))).To(Succeed())
			Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(uuid.NewString()))).To(Succeed())

			last, err := repo.GetOrgSyncLastRun(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(last.HoldActive).To(BeTrue(), "an unrelated failure must not clear the hold")
			Expect(last.HoldRecordedAt).NotTo(BeNil())
			Expect(last.HoldThresholdPercent).NotTo(BeNil())
			Expect(*last.HoldThresholdPercent).To(Equal(20.0))
			Expect(last.LastAttempt.Status).To(Equal(orgprovider.StatusFailed))
			Expect(last.LastAttempt.ThresholdPercent).To(BeNil(), "the failed attempt has no threshold, which is why the hold's own is needed")
		})

		It("is cleared by the admin dismissal", func() {
			Expect(repo.RecordOrgSyncAttempt(context.Background(), blockedAttempt(uuid.NewString(), 20))).To(Succeed())
			Expect(repo.ClearOrgSyncHold(context.Background())).To(Succeed())

			last, err := repo.GetOrgSyncLastRun(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(last.HoldActive).To(BeFalse())
			Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(1), "dismissing resolves the hold; it does not delete the record")
		})

		It("is cleared by a later successful sync, and the success is its own row", func() {
			Expect(repo.RecordOrgSyncAttempt(context.Background(), blockedAttempt(uuid.NewString(), 20))).To(Succeed())

			_, err := newService("90").SyncWithOptions(context.Background(), services.SyncOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE hold_active`)).To(Equal(0))
			Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(2))
		})

		It("rejects a hold on a row that is not blocked", func() {
			_, err := db.Exec(`
				INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at, hold_active)
				VALUES (gen_random_uuid(), 'manual', 'failed', NOW(), NOW(), true)
			`)
			Expect(err).To(HaveOccurred())
		})
	})

	It("prunes the oldest rows beyond the cap on insert, keeping the newest", func() {
		// Seed well past the 1000-row cap in one statement, then let a real
		// insert trigger the prune.
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at)
			SELECT gen_random_uuid(), 'manual', 'failed', NOW(), NOW() FROM generate_series(1, 1005)
		`)
		Expect(err).NotTo(HaveOccurred())
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(1005))

		Expect(repo.RecordOrgSyncSkip(context.Background(), orgprovider.OrgSyncSkip{
			RunID: uuid.NewString(), Reason: orgprovider.SkipManualRunning, At: time.Now().UTC(),
		})).To(Succeed())

		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs`)).To(Equal(1000))
		Expect(countRows(`SELECT COUNT(*) FROM org_sync_runs WHERE status = 'skipped'`)).To(Equal(1),
			"the newest row must survive the prune")
	})

	It("rejects a duplicate run ID, so one ID names exactly one request", func() {
		id := uuid.NewString()
		Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(id))).To(Succeed())
		Expect(repo.RecordOrgSyncAttempt(context.Background(), failedAttempt(id))).NotTo(Succeed())
	})

	It("rejects an invalid status via the CHECK constraint", func() {
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at)
			VALUES (gen_random_uuid(), 'manual', 'exploded', NOW(), NOW())
		`)
		Expect(err).To(HaveOccurred())
	})

	It("rejects a skipped row with no skip_reason, and a skip_reason on a non-skipped row", func() {
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at)
			VALUES (gen_random_uuid(), 'scheduled', 'skipped', NOW(), NOW())
		`)
		Expect(err).To(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at, skip_reason)
			VALUES (gen_random_uuid(), 'scheduled', 'failed', NOW(), NOW(), 'manual_sync_running')
		`)
		Expect(err).To(HaveOccurred())
	})
	It("has no id column: the run ID is the primary key", func() {
		Expect(countRows(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'org_sync_runs' AND column_name = 'id'`)).To(Equal(0))

		var keyColumn string
		Expect(db.QueryRow(`
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
			WHERE tc.table_name = 'org_sync_runs' AND tc.constraint_type = 'PRIMARY KEY'
		`).Scan(&keyColumn)).To(Succeed())
		Expect(keyColumn).To(Equal("run_id"))
	})

	It("orders by when a request finished, not by when its row was inserted", func() {
		older, newer := uuid.NewString(), uuid.NewString()
		// Insert the NEWER sync first, then the older one: insertion order is
		// the opposite of finish order, so only a finished_at ordering passes.
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at)
			VALUES ($1, 'manual', 'failed', NOW(), NOW())
		`, newer)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at)
			VALUES ($1, 'manual', 'failed', NOW() - interval '1 hour', NOW() - interval '1 hour')
		`, older)
		Expect(err).NotTo(HaveOccurred())

		entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries[0].RunID).To(Equal(newer))
		Expect(entries[1].RunID).To(Equal(older))

		var top string
		Expect(db.QueryRow(`SELECT run_id::text FROM org_sync_runs_latest LIMIT 1`).Scan(&top)).To(Succeed())
		Expect(top).To(Equal(newer))
	})

	It("breaks a finished_at tie deterministically, so the order never flips between reads", func() {
		_, err := db.Exec(`
			INSERT INTO org_sync_runs (run_id, trigger, status, started_at, finished_at, created_at) VALUES
			  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'manual', 'failed', '2026-01-01 00:00:00+00', '2026-01-01 00:00:00+00', '2026-01-01 00:00:00+00'),
			  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'manual', 'failed', '2026-01-01 00:00:00+00', '2026-01-01 00:00:00+00', '2026-01-01 00:00:00+00')
		`)
		Expect(err).NotTo(HaveOccurred())

		for i := 0; i < 3; i++ {
			entries, err := repo.ListOrgSyncHistory(context.Background(), 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries[0].RunID).To(Equal("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"))
			Expect(entries[1].RunID).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
		}
	})
})
