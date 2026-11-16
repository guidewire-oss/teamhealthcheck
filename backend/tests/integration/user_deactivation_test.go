package integration_test

import (
	"context"
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User soft deactivation and status listing", func() {
	var (
		db      *sql.DB
		cleanup func()
		repo    user.Repository
		ctx     = context.Background()
	)

	count := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		repo = postgres.NewUserRepository(db)

		for _, u := range []struct{ id, name string }{{"du-alice", "Alice"}, {"du-bob", "Bob"}} {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1::varchar, $1::varchar, $1::varchar || '@test.com', $2, 'level-5', '')`, u.id, u.name)
			Expect(err).NotTo(HaveOccurred())
		}
		_, err := db.Exec(`INSERT INTO teams (id, name) VALUES ('du-team', 'DU Team')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('du-team','du-bob')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
			VALUES ('du-session', 'du-team', 'du-bob', '2026-02-01', '2026 - 1st Half', true)`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cleanup() })

	It("deactivates a person without deleting them, ending memberships and keeping history", func() {
		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())

		Expect(count(`SELECT COUNT(*) FROM users WHERE id='du-bob' AND is_active = false AND deactivated_at IS NOT NULL`)).To(Equal(1),
			"the row must still exist, marked inactive")
		Expect(count(`SELECT COUNT(*) FROM team_members WHERE user_id='du-bob' AND team_id='du-team' AND is_active = false`)).To(Equal(1),
			"the membership is kept as inactive history")
		Expect(count(`SELECT COUNT(*) FROM health_check_sessions WHERE id='du-session' AND user_id='du-bob'`)).To(Equal(1),
			"past surveys are untouched")
	})

	It("is a no-op the second time and keeps the original deactivation time", func() {
		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())
		var first string
		Expect(db.QueryRow(`SELECT deactivated_at::text FROM users WHERE id='du-bob'`).Scan(&first)).To(Succeed())

		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())
		var second string
		Expect(db.QueryRow(`SELECT deactivated_at::text FROM users WHERE id='du-bob'`).Scan(&second)).To(Succeed())
		Expect(second).To(Equal(first))
	})

	It("reports an unknown person as not found", func() {
		err := repo.Deactivate(ctx, "du-nobody")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("user not found"))
	})

	It("lists inactive people by default, and filters by status", func() {
		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())

		ids := func(status string) []string {
			users, _, err := repo.FindPage(ctx, user.ListFilter{Search: "du-", Status: status}, 1, 50)
			Expect(err).NotTo(HaveOccurred())
			out := []string{}
			for _, u := range users {
				out = append(out, u.ID)
			}
			return out
		}
		Expect(ids("")).To(ConsistOf("du-alice", "du-bob"), "default listing keeps inactive people")
		Expect(ids(user.StatusActive)).To(ConsistOf("du-alice"))
		Expect(ids(user.StatusInactive)).To(ConsistOf("du-bob"))
	})

	It("keeps inactive people searchable and carries the status on the listed user", func() {
		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())
		users, total, err := repo.FindPage(ctx, user.ListFilter{Search: "bob"}, 1, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(users[0].IsActive).To(BeFalse())
		Expect(users[0].DeactivatedAt).NotTo(BeNil())

		alice, err := repo.FindByID(ctx, "du-alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(alice.IsActive).To(BeTrue())
	})

	It("blocks an inactive person from signing in", func() {
		Expect(repo.Deactivate(ctx, "du-bob")).To(Succeed())
		_, err := repo.FindByUsername(ctx, "du-bob")
		Expect(err).To(HaveOccurred())
	})
})
