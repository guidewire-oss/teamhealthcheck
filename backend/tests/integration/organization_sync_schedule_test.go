package integration_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Exercises the Phase 2 schedule persistence against a real database: the
// migration's defaults and CHECK constraint, and the atomic claim's
// compare-and-swap semantics under concurrent contention -- the one property
// that cannot be proven with a fake Store (a fake can be TOLD to simulate a
// race, but only a real database's row-level locking actually proves the
// SQL WHERE clause resolves a genuine race correctly).
var _ = Describe("Integration: Organization Sync Schedule", func() {
	var (
		db      *sql.DB
		cleanup func()
		repo    organization.Repository
	)

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		repo = postgres.NewOrganizationRepository(db)
	})

	AfterEach(func() {
		cleanup()
	})

	It("defaults to disabled with no frequency or next run", func() {
		schedule, err := repo.GetOrgSyncSchedule(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(schedule.Enabled).To(BeFalse())
		Expect(schedule.Frequency).To(BeEmpty())
		Expect(schedule.NextRunAt).To(BeNil())
	})

	It("rejects an invalid frequency via the CHECK constraint", func() {
		_, err := db.Exec(`UPDATE app_settings SET org_sync_schedule_frequency = 'hourly' WHERE id = 1`)
		Expect(err).To(HaveOccurred())
	})

	It("rejects enabling without a frequency and next_run_at via the completeness CHECK", func() {
		_, err := db.Exec(`UPDATE app_settings SET org_sync_schedule_enabled = true WHERE id = 1`)
		Expect(err).To(HaveOccurred(), "enabled=true with no frequency/next_run_at must violate org_sync_schedule_complete")
	})

	It("round-trips enabling through UpdateOrgSyncSchedule", func() {
		next := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
		Expect(repo.UpdateOrgSyncSchedule(context.Background(), true, "daily", &next)).To(Succeed())

		schedule, err := repo.GetOrgSyncSchedule(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(schedule.Enabled).To(BeTrue())
		Expect(schedule.Frequency).To(Equal("daily"))
		Expect(schedule.NextRunAt).NotTo(BeNil())
		Expect(schedule.NextRunAt.Equal(next)).To(BeTrue())
	})

	It("clears next_run_at on disable while UpdateOrgSyncSchedule is called with nil", func() {
		next := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
		Expect(repo.UpdateOrgSyncSchedule(context.Background(), true, "weekly", &next)).To(Succeed())
		Expect(repo.UpdateOrgSyncSchedule(context.Background(), false, "weekly", nil)).To(Succeed())

		schedule, err := repo.GetOrgSyncSchedule(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(schedule.Enabled).To(BeFalse())
		Expect(schedule.NextRunAt).To(BeNil())
	})

	Describe("ClaimOrgSyncOccurrence", func() {
		var due time.Time

		BeforeEach(func() {
			due = time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
			Expect(repo.UpdateOrgSyncSchedule(context.Background(), true, "daily", &due)).To(Succeed())
		})

		It("succeeds exactly once when the observed value matches, moving next_run_at", func() {
			newNext := due.Add(24 * time.Hour)
			won, err := repo.ClaimOrgSyncOccurrence(context.Background(), due, newNext)
			Expect(err).NotTo(HaveOccurred())
			Expect(won).To(BeTrue())

			schedule, err := repo.GetOrgSyncSchedule(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(schedule.NextRunAt.Equal(newNext)).To(BeTrue())
		})

		It("fails when the observed value is stale (another replica already claimed it)", func() {
			firstClaim := due.Add(24 * time.Hour)
			won1, err := repo.ClaimOrgSyncOccurrence(context.Background(), due, firstClaim)
			Expect(err).NotTo(HaveOccurred())
			Expect(won1).To(BeTrue())

			// A second claim using the SAME stale "observed" value must lose --
			// this is the actual compare-and-swap race the design depends on.
			won2, err := repo.ClaimOrgSyncOccurrence(context.Background(), due, due.Add(48*time.Hour))
			Expect(err).NotTo(HaveOccurred())
			Expect(won2).To(BeFalse(), "a stale observed value must never win a second claim")

			schedule, err := repo.GetOrgSyncSchedule(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(schedule.NextRunAt.Equal(firstClaim)).To(BeTrue(), "the second, losing claim must not have moved next_run_at again")
		})

		It("fails when the schedule was disabled in the interim, even with a matching observed value", func() {
			Expect(repo.UpdateOrgSyncSchedule(context.Background(), false, "daily", nil)).To(Succeed())

			won, err := repo.ClaimOrgSyncOccurrence(context.Background(), due, due.Add(24*time.Hour))
			Expect(err).NotTo(HaveOccurred())
			Expect(won).To(BeFalse(), "a claim must not succeed against a schedule an admin disabled mid-race")
		})

		It("exactly one of N concurrent claims against the same observed value wins", func() {
			const n = 8
			results := make(chan bool, n)
			for i := 0; i < n; i++ {
				go func(i int) {
					won, err := repo.ClaimOrgSyncOccurrence(context.Background(), due, due.Add(time.Duration(i+1)*time.Hour))
					Expect(err).NotTo(HaveOccurred())
					results <- won
				}(i)
			}
			wins := 0
			for i := 0; i < n; i++ {
				if <-results {
					wins++
				}
			}
			Expect(wins).To(Equal(1), "exactly one of N replicas racing the same due occurrence must win the claim")
		})
	})
})
