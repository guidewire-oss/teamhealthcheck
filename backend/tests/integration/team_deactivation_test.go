package integration_test

import (
	"context"
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/domain/team"
	"github.com/agopalakrishnan/teams360/backend/domain/user"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Team soft deactivation, status listing and membership history", func() {
	var (
		db       *sql.DB
		cleanup  func()
		teamRepo team.Repository
		userRepo user.Repository
		ctx      = context.Background()
	)

	count := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}
	membership := func(teamID, userID string, active bool) int {
		return count(`SELECT COUNT(*) FROM team_members WHERE team_id=$1 AND user_id=$2 AND is_active=$3`, teamID, userID, active)
	}

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		teamRepo = postgres.NewTeamRepository(db)
		userRepo = postgres.NewUserRepository(db)

		for _, id := range []string{"dt-alice", "dt-bob"} {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1::varchar, $1::varchar, $1::varchar || '@test.com', $1::varchar, 'level-5', '')`, id)
			Expect(err).NotTo(HaveOccurred())
		}
		for _, id := range []string{"dt-team-1", "dt-team-2"} {
			_, err := db.Exec(`INSERT INTO teams (id, name) VALUES ($1::varchar, $1::varchar)`, id)
			Expect(err).NotTo(HaveOccurred())
		}
		_, err := db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('dt-team-1','dt-alice'), ('dt-team-1','dt-bob')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
			VALUES ('dt-session', 'dt-team-1', 'dt-alice', '2026-02-01', '2026 - 1st Half', true)`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(`INSERT INTO action_items (id, team_id, created_by, title) VALUES ('dt-action','dt-team-1','dt-alice','Follow up')`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cleanup() })

	Describe("Deactivate", func() {
		It("keeps the team, ends its memberships, and keeps surveys and action items", func() {
			Expect(teamRepo.Deactivate(ctx, "dt-team-1")).To(Succeed())

			Expect(count(`SELECT COUNT(*) FROM teams WHERE id='dt-team-1' AND is_active=false AND deactivated_at IS NOT NULL`)).To(Equal(1),
				"the row must still exist, marked inactive")
			Expect(count(`SELECT COUNT(*) FROM team_members WHERE team_id='dt-team-1' AND is_active=false`)).To(Equal(2), "memberships kept as history")
			Expect(count(`SELECT COUNT(*) FROM health_check_sessions WHERE id='dt-session'`)).To(Equal(1))
			Expect(count(`SELECT COUNT(*) FROM action_items WHERE id='dt-action'`)).To(Equal(1))
		})

		It("is a no-op the second time and keeps the original deactivation time", func() {
			Expect(teamRepo.Deactivate(ctx, "dt-team-1")).To(Succeed())
			var first, second string
			Expect(db.QueryRow(`SELECT deactivated_at::text FROM teams WHERE id='dt-team-1'`).Scan(&first)).To(Succeed())
			Expect(teamRepo.Deactivate(ctx, "dt-team-1")).To(Succeed())
			Expect(db.QueryRow(`SELECT deactivated_at::text FROM teams WHERE id='dt-team-1'`).Scan(&second)).To(Succeed())
			Expect(second).To(Equal(first))
		})

		It("reports an unknown team as not found", func() {
			err := teamRepo.Deactivate(ctx, "dt-nobody")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team not found"))
		})
	})

	Describe("listing", func() {
		It("lists inactive teams alongside active ones and carries the status", func() {
			Expect(teamRepo.Deactivate(ctx, "dt-team-1")).To(Succeed())

			all, err := teamRepo.FindAllWithDetails(ctx)
			Expect(err).NotTo(HaveOccurred())
			status := map[string]bool{}
			members := map[string]int{}
			for _, t := range all {
				status[t.ID] = t.IsActive
				members[t.ID] = t.MemberCount
			}
			Expect(status).To(HaveKeyWithValue("dt-team-1", false))
			Expect(status).To(HaveKeyWithValue("dt-team-2", true))
			Expect(members["dt-team-1"]).To(BeZero(), "an inactive team has no current members")
		})
	})

	Describe("a team stays active when only its lead or members change", func() {
		It("does not deactivate on an admin edit that changes the lead and the member list", func() {
			t, err := teamRepo.FindByID(ctx, "dt-team-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(t.IsActive).To(BeTrue())

			lead := "dt-bob"
			t.TeamLeadID = &lead
			t.Name = "Renamed"
			t.Members = []team.TeamMember{{ID: "dt-bob"}} // alice leaves the team
			Expect(teamRepo.Update(ctx, t)).To(Succeed())

			Expect(count(`SELECT COUNT(*) FROM teams WHERE id='dt-team-1' AND is_active AND deactivated_at IS NULL`)).To(Equal(1),
				"changing the lead, name and members never makes a team inactive")
			Expect(membership("dt-team-1", "dt-alice", false)).To(Equal(1), "alice's membership is kept as inactive history, not deleted")
			Expect(membership("dt-team-1", "dt-bob", true)).To(Equal(1))
		})

		It("does not deactivate when a member is added or removed", func() {
			Expect(teamRepo.RemoveMember(ctx, "dt-team-1", "dt-bob")).To(Succeed())
			Expect(teamRepo.AddMember(ctx, "dt-team-2", "dt-bob")).To(Succeed())
			Expect(count(`SELECT COUNT(*) FROM teams WHERE id IN ('dt-team-1','dt-team-2') AND is_active`)).To(Equal(2))
		})
	})

	Describe("membership history is never destroyed by an ordinary edit", func() {
		It("RemoveMember ends the membership but keeps the row", func() {
			Expect(teamRepo.RemoveMember(ctx, "dt-team-1", "dt-bob")).To(Succeed())
			Expect(membership("dt-team-1", "dt-bob", false)).To(Equal(1))
			Expect(membership("dt-team-1", "dt-bob", true)).To(BeZero())
		})

		It("AddMember reactivates a membership that had ended", func() {
			Expect(teamRepo.RemoveMember(ctx, "dt-team-1", "dt-bob")).To(Succeed())
			Expect(teamRepo.AddMember(ctx, "dt-team-1", "dt-bob")).To(Succeed())
			Expect(membership("dt-team-1", "dt-bob", true)).To(Equal(1))
		})

		It("editing a team does not erase or resurrect an inactive membership", func() {
			Expect(teamRepo.RemoveMember(ctx, "dt-team-1", "dt-bob")).To(Succeed())

			t, err := teamRepo.FindByID(ctx, "dt-team-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Members).To(HaveLen(1), "only current members are loaded")
			t.Name = "Edited"
			Expect(teamRepo.Update(ctx, t)).To(Succeed())

			Expect(membership("dt-team-1", "dt-bob", false)).To(Equal(1), "bob's inactive history survives the edit")
			Expect(membership("dt-team-1", "dt-bob", true)).To(BeZero(), "and he is not resurrected")
		})

		It("moving a person between teams keeps the old membership as history, and a later edit does not resurrect it", func() {
			// Himani-style move: alice leaves team-1 for team-2.
			alice, err := userRepo.FindByID(ctx, "dt-alice")
			Expect(err).NotTo(HaveOccurred())
			alice.TeamIDs = []string{"dt-team-2"}
			Expect(userRepo.Update(ctx, alice)).To(Succeed())

			Expect(membership("dt-team-1", "dt-alice", false)).To(Equal(1), "old team kept as inactive history")
			Expect(membership("dt-team-2", "dt-alice", true)).To(Equal(1))
			Expect(count(`SELECT COUNT(*) FROM health_check_sessions WHERE id='dt-session' AND team_id='dt-team-1'`)).To(Equal(1), "the old survey stays under the old team")

			// An unrelated edit (rename) must not bring the old membership back.
			again, err := userRepo.FindByID(ctx, "dt-alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(again.TeamIDs).To(ConsistOf("dt-team-2"), "a person's teams are their current ones only")
			again.Name = "Alice Renamed"
			Expect(userRepo.Update(ctx, again)).To(Succeed())

			Expect(membership("dt-team-1", "dt-alice", true)).To(BeZero(), "the old membership must not be resurrected by an edit")
			Expect(membership("dt-team-1", "dt-alice", false)).To(Equal(1))
		})
	})
})
