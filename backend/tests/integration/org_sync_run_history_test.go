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
})
