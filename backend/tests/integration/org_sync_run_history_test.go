package integration_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Org sync run history (real Postgres)", func() {
	var (
		db      *sql.DB
		cleanup func()
		repo    *postgres.OrgSyncRunRepository
		ctx     context.Context
		base    time.Time
	)

	insert := func(id, status, trigger string, age time.Duration) {
		_, err := db.Exec(`INSERT INTO org_sync_runs (id, status, stage, initiated_by, snapshot_type, trigger_type, created_at)
			VALUES ($1, $2, 'QUEUED', 'admin', 'COMPLETE', $3, $4)`, id, status, trigger, base.Add(age))
		Expect(err).NotTo(HaveOccurred())
	}
	ids := func(runs []orgsyncrun.Run) []string {
		out := []string{}
		for _, r := range runs {
			out = append(out, r.ID)
		}
		return out
	}

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		repo = postgres.NewOrgSyncRunRepository(db)
		ctx = context.Background()
		base = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
		// Terminal rows only: the single-active index allows one PENDING/RUNNING.
		insert("r1", "SUCCESS", "MANUAL", 0)
		insert("r2", "FAILED", "MANUAL", time.Hour)
		insert("r3", "SUCCESS", "SCHEDULED", 2*time.Hour)
		insert("r4", "FAILED", "SCHEDULED", 3*time.Hour)
		insert("r5", "RUNNING", "MANUAL", 4*time.Hour)
	})

	AfterEach(func() { cleanup() })

	It("lists newest first with the total", func() {
		runs, total, err := repo.List(ctx, orgsyncrun.HistoryFilter{Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(5))
		Expect(ids(runs)).To(Equal([]string{"r5", "r4", "r3", "r2", "r1"}))
	})

	It("paginates with limit and offset while total stays the full match count", func() {
		runs, total, err := repo.List(ctx, orgsyncrun.HistoryFilter{Limit: 2, Offset: 2})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(5))
		Expect(ids(runs)).To(Equal([]string{"r3", "r2"}))

		runs, _, err = repo.List(ctx, orgsyncrun.HistoryFilter{Limit: 2, Offset: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(runs).To(BeEmpty())
	})

	It("filters by status, trigger and snapshot type", func() {
		runs, total, err := repo.List(ctx, orgsyncrun.HistoryFilter{Status: orgsyncrun.StatusFailed, Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		Expect(ids(runs)).To(Equal([]string{"r4", "r2"}))

		runs, _, err = repo.List(ctx, orgsyncrun.HistoryFilter{Status: orgsyncrun.StatusFailed, Trigger: orgsyncrun.TriggerScheduled, Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(ids(runs)).To(Equal([]string{"r4"}))

		runs, total, err = repo.List(ctx, orgsyncrun.HistoryFilter{SnapshotType: orgprovider.SnapshotPartial, Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(0))
		Expect(runs).To(BeEmpty())
	})

	It("returns the same rows the single-run read does (columns map correctly)", func() {
		runs, _, err := repo.List(ctx, orgsyncrun.HistoryFilter{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		one, err := repo.Get(ctx, runs[0].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(runs[0]).To(Equal(*one))
	})

	Describe("per-run added counts", func() {
		It("persists each run's own counts and keeps unchanged unknown (nil) distinct from 0", func() {
			// r1: recorded counts incl. a genuine 0 unchanged on one side.
			_, err := db.Exec(`UPDATE org_sync_runs SET inserted_users=2, updated_users=1, inserted_teams=1,
				updated_teams=0, membership_changes=3, unchanged_users=7, unchanged_teams=0, unchanged_recorded=TRUE WHERE id='r1'`)
			Expect(err).NotTo(HaveOccurred())
			// r3: committed zeros, unchanged never recorded (flag left FALSE).

			r1, err := repo.Get(ctx, "r1")
			Expect(err).NotTo(HaveOccurred())
			Expect([]int{r1.InsertedUsers, r1.UpdatedUsers, r1.InsertedTeams, r1.MembershipChanges}).To(Equal([]int{2, 1, 1, 3}))
			Expect(r1.UnchangedUsers).NotTo(BeNil())
			Expect(*r1.UnchangedUsers).To(Equal(7))
			Expect(r1.UnchangedTeams).NotTo(BeNil())
			Expect(*r1.UnchangedTeams).To(Equal(0))

			r3, err := repo.Get(ctx, "r3")
			Expect(err).NotTo(HaveOccurred())
			Expect(r3.InsertedUsers).To(Equal(0))
			Expect(r3.UnchangedUsers).To(BeNil())
			Expect(r3.UnchangedTeams).To(BeNil())
		})

		It("reports a genuine all-zero count as 0 once the run recorded it", func() {
			_, err := db.Exec(`UPDATE org_sync_runs SET unchanged_users=0, unchanged_teams=0, unchanged_recorded=TRUE WHERE id='r3'`)
			Expect(err).NotTo(HaveOccurred())
			r3, err := repo.Get(ctx, "r3")
			Expect(err).NotTo(HaveOccurred())
			Expect(r3.UnchangedUsers).NotTo(BeNil())
			Expect(*r3.UnchangedUsers).To(Equal(0))
			Expect(r3.UnchangedTeams).NotTo(BeNil())
			Expect(*r3.UnchangedTeams).To(Equal(0))
		})

		It("leaves unchanged unknown for a run created by the repository", func() {
			// r5 is RUNNING and the single-active index allows only one active run.
			_, err := db.Exec(`UPDATE org_sync_runs SET status='FAILED' WHERE id='r5'`)
			Expect(err).NotTo(HaveOccurred())

			_, _, err = repo.CreatePending(ctx, orgsyncrun.NewParams{
				ID: "r-new", InitiatedBy: "admin", SnapshotType: orgprovider.SnapshotComplete,
			})
			Expect(err).NotTo(HaveOccurred())

			created, err := repo.Get(ctx, "r-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(created.UnchangedUsers).To(BeNil())
			Expect(created.UnchangedTeams).To(BeNil())
		})

		It("never stores NULL in the count columns, so a pre-000032 replica can still scan every row", func() {
			// The previous release scans these columns into a plain int. A NULL would make
			// every read of that row fail, so none may exist, however the row was written.
			_, err := db.Exec(`UPDATE org_sync_runs SET status='FAILED' WHERE id='r5'`)
			Expect(err).NotTo(HaveOccurred())
			_, _, err = repo.CreatePending(ctx, orgsyncrun.NewParams{
				ID: "r-new", InitiatedBy: "admin", SnapshotType: orgprovider.SnapshotComplete,
			})
			Expect(err).NotTo(HaveOccurred())

			var nulls int
			Expect(db.QueryRow(`SELECT count(*) FROM org_sync_runs WHERE unchanged_users IS NULL OR unchanged_teams IS NULL`).Scan(&nulls)).To(Succeed())
			Expect(nulls).To(BeZero())
		})

		It("reads a row an older replica wrote, which cannot set the flag, as unknown not as 0", func() {
			// r2 was inserted with a plain INSERT that omits the unchanged columns and the flag.
			r2, err := repo.Get(ctx, "r2")
			Expect(err).NotTo(HaveOccurred())
			Expect(r2.UnchangedUsers).To(BeNil())
			Expect(r2.UnchangedTeams).To(BeNil())
		})
	})
})
