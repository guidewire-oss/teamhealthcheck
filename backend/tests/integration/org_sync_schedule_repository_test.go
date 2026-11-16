package integration_test

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Org sync schedule persistence (real Postgres)", func() {
	var (
		db       *sql.DB
		cleanup  func()
		schedule *postgres.OrgSyncScheduleRepository
		runs     *postgres.OrgSyncRunRepository
		ctx      context.Context
		due      time.Time
		next     time.Time
	)

	enable := func(at time.Time) {
		_, err := schedule.Save(ctx, true, orgsyncschedule.Daily, &at, "admin")
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		schedule = postgres.NewOrgSyncScheduleRepository(db)
		runs = postgres.NewOrgSyncRunRepository(db)
		ctx = context.Background()
		due = time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
		next = due.AddDate(0, 0, 1)
	})

	AfterEach(func() { cleanup() })

	Describe("configuration", func() {
		It("is seeded disabled with a single row and no next run", func() {
			s, err := schedule.Get(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Enabled).To(BeFalse())
			Expect(s.Frequency).To(Equal(orgsyncschedule.Daily))
			Expect(s.NextRunAt).To(BeNil())

			var rows int
			Expect(db.QueryRow(`SELECT COUNT(*) FROM org_sync_schedule`).Scan(&rows)).To(Succeed())
			Expect(rows).To(Equal(1))
			_, err = db.Exec(`INSERT INTO org_sync_schedule (id) VALUES (2)`)
			Expect(err).To(HaveOccurred(), "the singleton constraint must reject a second row")
		})

		It("round-trips enablement, frequency and next run in UTC, and clears next run when disabled", func() {
			_, err := schedule.Save(ctx, true, orgsyncschedule.Monthly, &due, "admin")
			Expect(err).NotTo(HaveOccurred())
			s, err := schedule.Get(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Enabled).To(BeTrue())
			Expect(s.Frequency).To(Equal(orgsyncschedule.Monthly))
			Expect(s.NextRunAt.Equal(due)).To(BeTrue())
			Expect(s.NextRunAt.Location()).To(Equal(time.UTC))
			Expect(*s.UpdatedBy).To(Equal("admin"))

			s, err = schedule.Save(ctx, false, orgsyncschedule.Monthly, &due, "admin")
			Expect(err).NotTo(HaveOccurred())
			Expect(s.Enabled).To(BeFalse())
			Expect(s.NextRunAt).To(BeNil())
		})

		It("rejects an unknown frequency at the database", func() {
			_, err := db.Exec(`UPDATE org_sync_schedule SET frequency = 'HOURLY' WHERE id = 1`)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Claim", func() {
		It("succeeds once and moves next_run_at forward", func() {
			enable(due)
			ok, err := schedule.Claim(ctx, due, next)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			s, _ := schedule.Get(ctx)
			Expect(s.NextRunAt.Equal(next)).To(BeTrue())
		})

		It("returns false when the same occurrence is claimed again", func() {
			enable(due)
			ok, _ := schedule.Claim(ctx, due, next)
			Expect(ok).To(BeTrue())
			ok, err := schedule.Claim(ctx, due, next.AddDate(0, 0, 1))
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
			s, _ := schedule.Get(ctx)
			Expect(s.NextRunAt.Equal(next)).To(BeTrue(), "a lost claim must not move next_run_at")
		})

		It("returns false after the schedule is disabled", func() {
			enable(due)
			_, err := schedule.Save(ctx, false, orgsyncschedule.Daily, nil, "admin")
			Expect(err).NotTo(HaveOccurred())
			ok, err := schedule.Claim(ctx, due, next)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("returns false when the occurrence was changed since it was read", func() {
			enable(due.AddDate(0, 0, 3))
			ok, err := schedule.Claim(ctx, due, next)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("lets exactly one of many concurrent claimers win", func() {
			enable(due)
			var wins int32
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					ok, err := schedule.Claim(ctx, due, next)
					Expect(err).NotTo(HaveOccurred())
					if ok {
						atomic.AddInt32(&wins, 1)
					}
				}()
			}
			wg.Wait()
			Expect(atomic.LoadInt32(&wins)).To(Equal(int32(1)))
		})
	})

	Describe("automatic-run history", func() {
		create := func(id string, trigger orgsyncrun.Trigger) {
			_, _, err := runs.CreatePending(ctx, orgsyncrun.NewParams{
				ID: id, InitiatedBy: "scheduler", SnapshotType: orgprovider.SnapshotComplete, Trigger: trigger,
			})
			Expect(err).NotTo(HaveOccurred())
		}
		finish := func(id, status string) {
			_, err := db.Exec(`UPDATE org_sync_runs SET status = $2, completed_at = CURRENT_TIMESTAMP WHERE id = $1`, id, status)
			Expect(err).NotTo(HaveOccurred())
		}

		It("defaults to MANUAL and keeps manual and scheduled runs separate", func() {
			Expect(runs.LatestByTrigger(ctx, orgsyncrun.TriggerScheduled)).To(BeNil())

			create("sync_manual", "")
			finish("sync_manual", "SUCCESS")
			manual, err := runs.Get(ctx, "sync_manual")
			Expect(err).NotTo(HaveOccurred())
			Expect(manual.Trigger).To(Equal(orgsyncrun.TriggerManual))
			Expect(runs.LatestByTrigger(ctx, orgsyncrun.TriggerScheduled)).To(BeNil(), "a manual run is not an automatic attempt")
			Expect(runs.LatestSuccessByTrigger(ctx, orgsyncrun.TriggerScheduled)).To(BeNil())
		})

		It("reports the latest scheduled attempt separately from the latest scheduled success", func() {
			create("sync_auto_1", orgsyncrun.TriggerScheduled)
			finish("sync_auto_1", "SUCCESS")
			time.Sleep(10 * time.Millisecond)
			create("sync_auto_2", orgsyncrun.TriggerScheduled)
			finish("sync_auto_2", "FAILED")

			last, err := runs.LatestByTrigger(ctx, orgsyncrun.TriggerScheduled)
			Expect(err).NotTo(HaveOccurred())
			Expect(last.ID).To(Equal("sync_auto_2"))
			Expect(last.Status).To(Equal(orgsyncrun.StatusFailed))

			ok, err := runs.LatestSuccessByTrigger(ctx, orgsyncrun.TriggerScheduled)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok.ID).To(Equal("sync_auto_1"), "a failed attempt must not count as the last success")
		})

		It("still enforces a single active run across manual and scheduled triggers", func() {
			create("sync_manual", orgsyncrun.TriggerManual)
			_, _, err := runs.CreatePending(ctx, orgsyncrun.NewParams{
				ID: "sync_auto", InitiatedBy: "scheduler", SnapshotType: orgprovider.SnapshotComplete, Trigger: orgsyncrun.TriggerScheduled,
			})
			Expect(err).To(MatchError(orgsyncrun.ErrActiveRunExists))
		})
	})

	Describe("rolling back 000031", func() {
		rollback := func() error {
			sqlText, err := os.ReadFile("../../infrastructure/persistence/postgres/migrations/000031_org_sync_schedule.down.sql")
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(string(sqlText))
			return err
		}

		It("refuses while auto sync is enabled", func() {
			enable(due)
			Expect(rollback()).To(MatchError(ContainSubstring("auto sync is enabled")))
		})

		It("refuses while automatic runs exist", func() {
			_, _, err := runs.CreatePending(ctx, orgsyncrun.NewParams{
				ID: "sync_auto", InitiatedBy: "scheduler", SnapshotType: orgprovider.SnapshotComplete, Trigger: orgsyncrun.TriggerScheduled,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rollback()).To(MatchError(ContainSubstring("automatic runs exist")))
		})

		It("succeeds when disabled and no automatic runs exist", func() {
			Expect(rollback()).To(Succeed())
		})
	})
})
